package app_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"receh/internal/app"
	"receh/internal/domain"
	"receh/internal/repository/mysql"

	driver "github.com/go-sql-driver/mysql"
	"golang.org/x/crypto/bcrypt"
)

// fakeClock lets a test move "today" so the recurring engine can be exercised
// across months without waiting.
type fakeClock struct {
	mu  sync.Mutex
	day string
}

func (c *fakeClock) Today() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.day
}

func (c *fakeClock) Now() time.Time {
	d, _ := domain.ParseDate(c.Today())
	return d.Add(9 * time.Hour)
}

func (c *fakeClock) set(day string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.day = day
}

// testServerDSN points at a MySQL server the tests may create databases on.
// Override with TEST_DB_DSN (no database name), e.g.
// "user:pass@tcp(127.0.0.1:3306)/".
func testServerDSN() string {
	if dsn := os.Getenv("TEST_DB_DSN"); dsn != "" {
		return dsn
	}
	return "root@tcp(127.0.0.1:3306)/"
}

var dbCounter atomic.Int64

// testDB gives each test its own fresh database and drops it afterwards, so
// tests never see each other's rows. It skips, loudly, when no server answers.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	cfg, err := driver.ParseDSN(testServerDSN())
	if err != nil {
		t.Fatalf("TEST_DB_DSN: %v", err)
	}
	admin, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	if err := admin.Ping(); err != nil {
		t.Skipf("MySQL tidak terjangkau di %s (%v). Nyalakan MySQL atau set TEST_DB_DSN.", cfg.Addr, err)
	}
	name := fmt.Sprintf("receh_test_%d_%d", time.Now().UnixNano()%1_000_000_000, dbCounter.Add(1))
	cfg.DBName = name
	db, err := mysql.Open(cfg.FormatDSN(), mysql.Options{MaxOpenConns: 20})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		if _, err := admin.Exec("DROP DATABASE IF EXISTS `" + name + "`"); err != nil {
			t.Logf("drop %s: %v", name, err)
		}
	})
	return db
}

type env struct {
	t     *testing.T
	srv   *httptest.Server
	app   *app.App
	clock *fakeClock
}

const adminEmail, adminPass = "admin@test.local", "admin-password-1"

func newEnv(t *testing.T, opts ...func(*app.Params)) *env {
	t.Helper()
	db := testDB(t)
	clock := &fakeClock{day: "2026-01-15"}
	p := app.Params{DB: db, Clock: clock, BcryptCost: bcrypt.MinCost,
		SessionCacheTTL: time.Minute, DashboardCacheTTL: time.Minute}
	for _, o := range opts {
		o(&p)
	}
	a := app.New(p)
	if _, err := a.Admin.EnsureAdmin(context.Background(), adminEmail, adminPass); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(a.Handler)
	t.Cleanup(func() {
		srv.Close()
		a.Close()
		db.Close()
	})
	return &env{t: t, srv: srv, app: a, clock: clock}
}

type client struct {
	e  *env
	hc *http.Client
	ua string
}

func (e *env) client(ua string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{e: e, hc: &http.Client{Jar: jar}, ua: ua}
}

func (c *client) do(method, path string, body any) (int, []byte) {
	c.e.t.Helper()
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.e.srv.URL+path, r)
	if err != nil {
		c.e.t.Fatal(err)
	}
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

// must asserts the status and decodes the body into a new T.
func must[T any](c *client, want int, method, path string, body any) T {
	c.e.t.Helper()
	status, data := c.do(method, path, body)
	if status != want {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, data)
	}
	var out T
	if len(data) > 0 {
		if err := json.Unmarshal(data, &out); err != nil {
			c.e.t.Fatalf("%s %s: decode: %v: %s", method, path, err, data)
		}
	}
	return out
}

func (c *client) expect(want int, method, path string, body any) string {
	c.e.t.Helper()
	status, data := c.do(method, path, body)
	if status != want {
		c.e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, status, want, data)
	}
	return string(data)
}

type M = map[string]any

var emailSeq atomic.Int64

func (e *env) newUser(ua string) *client {
	c := e.client(ua)
	c.expect(201, "POST", "/api/auth/register", M{
		"name": "Budi", "email": fmt.Sprintf("u%d@test.local", emailSeq.Add(1)), "password": "password-123",
	})
	return c
}

func (e *env) admin() *client {
	c := e.client("Mozilla/5.0 (Windows NT 10.0) Chrome/120.0")
	c.expect(200, "POST", "/api/auth/login", M{"email": adminEmail, "password": adminPass})
	return c
}

func (c *client) wallet(name string, balance int64) domain.Wallet {
	return must[domain.Wallet](c, 201, "POST", "/api/wallets", M{"name": name, "type": "ewallet", "color": "c1", "balance": balance})
}

func (c *client) category(typ domain.TxType, name string) int64 {
	for _, cat := range must[[]domain.Category](c, 200, "GET", "/api/categories", nil) {
		if cat.Type == typ && cat.Name == name {
			return cat.ID
		}
	}
	c.e.t.Fatalf("category %s not found", name)
	return 0
}

func (c *client) balance(id int64) int64 {
	c.e.t.Helper()
	for _, w := range must[[]domain.Wallet](c, 200, "GET", "/api/wallets", nil) {
		if w.ID == id {
			return w.Balance
		}
	}
	c.e.t.Fatalf("wallet %d not found", id)
	return 0
}

func txBody(walletID, catID int64, typ string, amount int64, on string) M {
	return M{"walletId": walletID, "categoryId": catID, "type": typ, "name": "Kegiatan", "amount": amount, "occurredOn": on}
}

func TestBalanceFollowsEveryTransactionChange(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 100_000)
	other := c.wallet("GoPay", 0)
	makan := c.category(domain.TxExpense, "Makan")
	saku := c.category(domain.TxIncome, "Uang Saku")

	spent := must[domain.Transaction](c, 201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 30_000, "2026-01-10"))
	if got := c.balance(w.ID); got != 70_000 {
		t.Fatalf("after expense: %d", got)
	}
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, saku, "income", 50_000, "2026-01-11"))
	if got := c.balance(w.ID); got != 120_000 {
		t.Fatalf("after income: %d", got)
	}
	// Editing the amount applies only the difference.
	c.expect(200, "PUT", fmt.Sprintf("/api/transactions/%d", spent.ID), txBody(w.ID, makan, "expense", 10_000, "2026-01-10"))
	if got := c.balance(w.ID); got != 140_000 {
		t.Fatalf("after edit: %d", got)
	}
	// Moving an income to another wallet moves the money with it.
	list := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions?type=income", nil)
	c.expect(200, "PUT", fmt.Sprintf("/api/transactions/%d", list.Items[0].ID), txBody(other.ID, saku, "income", 50_000, "2026-01-11"))
	if a, b := c.balance(w.ID), c.balance(other.ID); a != 90_000 || b != 50_000 {
		t.Fatalf("after move: %d %d", a, b)
	}
	// Deleting restores the balance.
	c.expect(204, "DELETE", fmt.Sprintf("/api/transactions/%d", spent.ID), nil)
	if got := c.balance(w.ID); got != 100_000 {
		t.Fatalf("after delete: %d", got)
	}
}

func TestOverdraftAndSpentIncomeAreRefused(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("DANA", 10_000)
	makan := c.category(domain.TxExpense, "Makan")
	saku := c.category(domain.TxIncome, "Uang Saku")

	c.expect(409, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 10_001, "2026-01-10"))
	if got := c.balance(w.ID); got != 10_000 {
		t.Fatalf("refused expense moved money: %d", got)
	}
	in := must[domain.Transaction](c, 201, "POST", "/api/transactions", txBody(w.ID, saku, "income", 5_000, "2026-01-10"))
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 15_000, "2026-01-10"))
	// That income has been spent, so removing it would make the balance negative.
	c.expect(409, "DELETE", fmt.Sprintf("/api/transactions/%d", in.ID), nil)
	if got := c.balance(w.ID); got != 0 {
		t.Fatalf("balance %d", got)
	}
}

func TestValidationRejectsBadInput(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("Tunai", 1000)
	makan := c.category(domain.TxExpense, "Makan")
	saku := c.category(domain.TxIncome, "Uang Saku")
	cases := []M{
		txBody(w.ID, makan, "expense", 0, "2026-01-10"),
		txBody(w.ID, makan, "expense", -5, "2026-01-10"),
		txBody(w.ID, saku, "expense", 100, "2026-01-10"),  // Category of the wrong type.
		txBody(w.ID, makan, "expense", 100, "2026-03-01"), // Future date.
		txBody(w.ID, makan, "expense", 100, "not-a-date"),
		txBody(w.ID, makan, "bonus", 100, "2026-01-10"),
		txBody(999999, makan, "expense", 100, "2026-01-10"),
		{"walletId": w.ID, "categoryId": makan, "type": "expense", "name": "   ", "amount": 100, "occurredOn": "2026-01-10"},
	}
	for i, body := range cases {
		c.expect(400, "POST", "/api/transactions", body)
		_ = i
	}
	if got := c.balance(w.ID); got != 1000 {
		t.Fatalf("balance %d", got)
	}
}

// The core race: 100 simultaneous spends of 1.000 from a wallet holding 50.000.
// Exactly 50 may succeed and the balance must land on exactly zero.
func TestConcurrentSpendingNeverOverdrawsOrLosesUpdates(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 50_000)
	makan := c.category(domain.TxExpense, "Makan")

	var ok, refused atomic.Int64
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, _ := c.do("POST", "/api/transactions", txBody(w.ID, makan, "expense", 1_000, "2026-01-10"))
			switch status {
			case 201:
				ok.Add(1)
			case 409:
				refused.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 50 || refused.Load() != 50 {
		t.Fatalf("ok=%d refused=%d, want 50/50", ok.Load(), refused.Load())
	}
	if got := c.balance(w.ID); got != 0 {
		t.Fatalf("balance %d, want 0", got)
	}
}

func TestConcurrentMixedTrafficKeepsLedgerConsistent(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	a, b := c.wallet("A", 500_000), c.wallet("B", 500_000)
	makan := c.category(domain.TxExpense, "Makan")
	saku := c.category(domain.TxIncome, "Uang Saku")

	var wg sync.WaitGroup
	for i := range 60 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wallet, cat, typ := a.ID, makan, "expense"
			if i%2 == 0 {
				wallet = b.ID
			}
			if i%3 == 0 {
				cat, typ = saku, "income"
			}
			c.do("POST", "/api/transactions", txBody(wallet, cat, typ, int64(1000+i), "2026-01-10"))
		}()
	}
	wg.Wait()

	// The stored balance must equal the opening balance plus the sum of history.
	sum := map[int64]int64{a.ID: 500_000, b.ID: 500_000}
	page := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions?pageSize=100", nil)
	if page.Total != 60 {
		t.Fatalf("recorded %d of 60", page.Total)
	}
	for _, tx := range page.Items {
		sum[tx.WalletID] += tx.Type.Delta(tx.Amount)
	}
	if c.balance(a.ID) != sum[a.ID] || c.balance(b.ID) != sum[b.ID] {
		t.Fatalf("balances diverge from ledger: %d/%d vs %d/%d", c.balance(a.ID), c.balance(b.ID), sum[a.ID], sum[b.ID])
	}
}

func TestUsersCannotReachEachOthersData(t *testing.T) {
	e := newEnv(t)
	alice, mallory := e.newUser(""), e.newUser("")
	w := alice.wallet("BCA", 10_000)
	cat := alice.category(domain.TxExpense, "Makan")
	tx := must[domain.Transaction](alice, 201, "POST", "/api/transactions", txBody(w.ID, cat, "expense", 1_000, "2026-01-10"))

	mallory.expect(404, "DELETE", fmt.Sprintf("/api/wallets/%d", w.ID), nil)
	mallory.expect(404, "DELETE", fmt.Sprintf("/api/transactions/%d", tx.ID), nil)
	mallory.expect(404, "DELETE", fmt.Sprintf("/api/categories/%d", cat), nil)
	mallory.expect(400, "POST", "/api/transactions", txBody(w.ID, cat, "expense", 1, "2026-01-10"))
	if p := must[domain.Page[domain.Transaction]](mallory, 200, "GET", "/api/transactions", nil); p.Total != 0 {
		t.Fatalf("mallory sees %d transactions", p.Total)
	}
	aliceSessions := must[[]domain.Session](alice, 200, "GET", "/api/sessions", nil)
	mallory.expect(404, "DELETE", fmt.Sprintf("/api/sessions/%d", aliceSessions[0].ID), nil)
}

func TestRecurringPostsMonthlyClampsAndCatchesUp(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 1_000_000)
	cat := c.category(domain.TxExpense, "Langganan")
	rule := must[domain.Recurring](c, 201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": cat, "type": "expense", "name": "Netflix",
		"amount": 50_000, "frequency": "monthly", "startOn": "2026-01-31",
	})
	if rule.NextDue != "2026-01-31" || c.balance(w.ID) != 1_000_000 {
		t.Fatalf("future rule posted early: %+v", rule)
	}
	sweep := func() {
		t.Helper()
		if _, err := e.app.Recurring.RunDue(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	sweep()
	if c.balance(w.ID) != 1_000_000 {
		t.Fatal("posted before due")
	}

	// The server was off for three months: catch up on all three, once each.
	e.clock.set("2026-03-31")
	sweep()
	sweep() // A second sweep must change nothing.
	if got := c.balance(w.ID); got != 850_000 {
		t.Fatalf("balance %d, want 850000", got)
	}
	page := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil)
	var dates []string
	for _, tx := range page.Items {
		dates = append(dates, tx.OccurredOn)
	}
	// Jan 31, then clamped to Feb 28, then back to Mar 31.
	if strings.Join(dates, ",") != "2026-03-31,2026-02-28,2026-01-31" {
		t.Fatalf("dates %v", dates)
	}
	if page.Items[0].Name != "Netflix" || page.Items[0].RecurringID == nil {
		t.Fatalf("bad posted tx: %+v", page.Items[0])
	}
}

func TestRecurringStartingTodayPostsImmediately(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 100_000)
	cat := c.category(domain.TxExpense, "Listrik")
	rule := must[domain.Recurring](c, 201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": cat, "type": "expense", "name": "Listrik kos",
		"amount": 50_000, "frequency": "monthly", "startOn": e.clock.Today(),
	})
	if c.balance(w.ID) != 50_000 || rule.NextDue != "2026-02-15" {
		t.Fatalf("balance %d next %s", c.balance(w.ID), rule.NextDue)
	}
	c.expect(400, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": cat, "type": "expense", "name": "Lewat",
		"amount": 1, "frequency": "monthly", "startOn": "2020-01-01",
	})
}

func TestRecurringWaitsForMoneyThenRetries(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("GoPay", 1_000)
	cat := c.category(domain.TxExpense, "Langganan")
	saku := c.category(domain.TxIncome, "Uang Saku")
	rule := must[domain.Recurring](c, 201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": cat, "type": "expense", "name": "Spotify",
		"amount": 5_000, "frequency": "monthly", "startOn": e.clock.Today(),
	})
	if rule.LastError == "" || rule.NextDue != "2026-01-15" || c.balance(w.ID) != 1_000 {
		t.Fatalf("should wait with a note: %+v balance %d", rule, c.balance(w.ID))
	}
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, saku, "income", 10_000, "2026-01-15"))
	if _, err := e.app.Recurring.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	if c.balance(w.ID) != 6_000 {
		t.Fatalf("balance %d, want 6000", c.balance(w.ID))
	}
	list := must[[]domain.Recurring](c, 200, "GET", "/api/recurring", nil)
	if list[0].LastError != "" || list[0].NextDue != "2026-02-15" {
		t.Fatalf("after retry: %+v", list[0])
	}
}

func TestPausedRuleSkipsMissedPeriodsOnResume(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 500_000)
	cat := c.category(domain.TxExpense, "Langganan")
	rule := must[domain.Recurring](c, 201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": cat, "type": "expense", "name": "Netflix",
		"amount": 50_000, "frequency": "monthly", "startOn": "2026-01-20",
	})
	path := fmt.Sprintf("/api/recurring/%d/active", rule.ID)
	c.expect(200, "POST", path, M{"active": false})
	e.clock.set("2026-04-10")
	if _, err := e.app.Recurring.RunDue(context.Background()); err != nil {
		t.Fatal(err)
	}
	resumed := must[domain.Recurring](c, 200, "POST", path, M{"active": true})
	if resumed.NextDue != "2026-04-20" || c.balance(w.ID) != 500_000 {
		t.Fatalf("resume charged or misdated: %+v balance %d", resumed, c.balance(w.ID))
	}
}

func TestConcurrentRecurringAndManualSpendStayConsistent(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 10_000)
	cat := c.category(domain.TxExpense, "Langganan")
	for i := range 8 {
		c.expect(201, "POST", "/api/recurring", M{
			"walletId": w.ID, "categoryId": cat, "type": "expense", "name": fmt.Sprintf("Rutin %d", i),
			"amount": 2_000, "frequency": "monthly", "startOn": "2026-01-20",
		})
	}
	e.clock.set("2026-01-20")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_, _ = e.app.Recurring.RunDue(context.Background())
	}()
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.do("POST", "/api/transactions", txBody(w.ID, cat, "expense", 2_000, "2026-01-20"))
		}()
	}
	wg.Wait()
	page := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions?pageSize=100", nil)
	if page.Total != 5 || c.balance(w.ID) != 0 {
		t.Fatalf("posted %d, balance %d; want 5 posts and 0", page.Total, c.balance(w.ID))
	}
}

func TestDashboardBucketsAndFilters(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	bca, gopay := c.wallet("BCA", 1_000_000), c.wallet("GoPay", 100_000)
	makan := c.category(domain.TxExpense, "Makan")
	listrik := c.category(domain.TxExpense, "Listrik")
	saku := c.category(domain.TxIncome, "Uang Saku")
	c.expect(201, "POST", "/api/transactions", txBody(bca.ID, makan, "expense", 20_000, "2026-01-15"))
	c.expect(201, "POST", "/api/transactions", txBody(gopay.ID, listrik, "expense", 50_000, "2026-01-14"))
	c.expect(201, "POST", "/api/transactions", txBody(bca.ID, saku, "income", 300_000, "2025-12-20"))

	d := must[domain.Dashboard](c, 200, "GET", "/api/dashboard?unit=month", nil)
	if len(d.Buckets) != 12 || d.Totals.Income != 300_000 || d.Totals.Expense != 70_000 {
		t.Fatalf("month: %+v", d.Totals)
	}
	last := d.Buckets[len(d.Buckets)-1]
	if last.Label != "Jan" || last.Expense != 70_000 || d.Buckets[len(d.Buckets)-2].Income != 300_000 {
		t.Fatalf("buckets: %+v", d.Buckets[10:])
	}
	if d.ExpenseSlices[0].Name != "Listrik" || d.ExpenseSlices[0].Total != 50_000 {
		t.Fatalf("slices: %+v", d.ExpenseSlices)
	}
	if d.TotalBalance != 1_000_000-20_000+300_000+100_000-50_000 {
		t.Fatalf("total balance %d", d.TotalBalance)
	}

	// Wallet filter scopes every figure.
	f := must[domain.Dashboard](c, 200, "GET", fmt.Sprintf("/api/dashboard?unit=month&walletId=%d", gopay.ID), nil)
	if f.Totals.Expense != 50_000 || f.Totals.Income != 0 || f.TotalBalance != 50_000 || len(f.Latest) != 1 {
		t.Fatalf("wallet filter: %+v", f.Totals)
	}
	// Every unit has its expected window, and an older window is reachable.
	for unit, n := range map[string]int{"day": 14, "week": 12, "month": 12, "year": 5} {
		if d := must[domain.Dashboard](c, 200, "GET", "/api/dashboard?unit="+unit, nil); len(d.Buckets) != n {
			t.Fatalf("%s: %d buckets", unit, len(d.Buckets))
		}
	}
	old := must[domain.Dashboard](c, 200, "GET", "/api/dashboard?unit=month&offset=1", nil)
	if old.Totals.Expense != 0 || !old.CanGoForward {
		t.Fatalf("offset window: %+v", old.Totals)
	}
	c.expect(400, "GET", "/api/dashboard?unit=decade", nil)

	// A bar click asks for one bucket's breakdown.
	s := must[struct {
		Totals domain.Totals
	}](c, 200, "GET", "/api/dashboard/slices?from=2026-01-15&to=2026-01-15", nil)
	if s.Totals.Expense != 20_000 {
		t.Fatalf("slices range: %+v", s.Totals)
	}
}

func TestMonthlyPDFIsValidAndComplete(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 5_000_000)
	makan := c.category(domain.TxExpense, "Makan")
	for i := range 90 { // Enough rows to force extra pages.
		c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", int64(1000+i), "2026-01-10"))
	}
	rep := must[domain.MonthlyReport](c, 200, "GET", "/api/reports/monthly?month=2026-01", nil)
	if len(rep.Transactions) != 90 || rep.Title != "Januari 2026" {
		t.Fatalf("report: %d %s", len(rep.Transactions), rep.Title)
	}

	status, data := c.do("GET", "/api/reports/monthly.pdf?month=2026-01", nil)
	if status != 200 {
		t.Fatalf("pdf status %d", status)
	}
	if !bytes.HasPrefix(data, []byte("%PDF-1.4")) || !bytes.HasSuffix(bytes.TrimSpace(data), []byte("%%EOF")) {
		t.Fatal("not a complete PDF")
	}
	if n := bytes.Count(data, []byte("/Type /Page ")); n < 2 {
		t.Fatalf("expected multiple pages, got %d", n)
	}
	// An empty month still renders.
	if s, d := c.do("GET", "/api/reports/monthly.pdf?month=2020-02", nil); s != 200 || !bytes.HasPrefix(d, []byte("%PDF")) {
		t.Fatalf("empty month: %d", s)
	}
	c.expect(400, "GET", "/api/reports/monthly.pdf?month=bogus", nil)
}

func TestDevicesAreListedAndCanBeSignedOut(t *testing.T) {
	e := newEnv(t)
	email := "dua@test.local"
	laptop := e.client("Mozilla/5.0 (Windows NT 10.0; Win64) AppleWebKit/537.36 Chrome/120.0 Safari/537.36")
	laptop.expect(201, "POST", "/api/auth/register", M{"name": "Dua", "email": email, "password": "password-123"})
	phone := e.client("Mozilla/5.0 (Linux; Android 14) AppleWebKit/537.36 Chrome/120.0 Mobile Safari/537.36")
	phone.expect(200, "POST", "/api/auth/login", M{"email": email, "password": "password-123"})

	list := must[[]domain.Session](laptop, 200, "GET", "/api/sessions", nil)
	if len(list) != 2 {
		t.Fatalf("%d sessions", len(list))
	}
	devices := map[string]bool{}
	var currentCount int
	var phoneID int64
	for _, s := range list {
		devices[s.Device] = true
		if s.Current {
			currentCount++
		} else {
			phoneID = s.ID
		}
	}
	if !devices["Chrome di Windows"] || !devices["Chrome di Android"] || currentCount != 1 {
		t.Fatalf("devices %v current %d", devices, currentCount)
	}

	// Signing the phone out takes effect on its very next request.
	phone.expect(200, "GET", "/api/auth/me", nil)
	laptop.expect(204, "DELETE", fmt.Sprintf("/api/sessions/%d", phoneID), nil)
	phone.expect(401, "GET", "/api/auth/me", nil)
	laptop.expect(200, "GET", "/api/auth/me", nil)
}

func TestChangingPasswordSignsOutOtherDevices(t *testing.T) {
	e := newEnv(t)
	email := "tiga@test.local"
	a := e.client("")
	a.expect(201, "POST", "/api/auth/register", M{"name": "Tiga", "email": email, "password": "password-123"})
	b := e.client("")
	b.expect(200, "POST", "/api/auth/login", M{"email": email, "password": "password-123"})

	a.expect(400, "POST", "/api/auth/password", M{"currentPassword": "wrong-password", "newPassword": "new-password-1"})
	a.expect(204, "POST", "/api/auth/password", M{"currentPassword": "password-123", "newPassword": "new-password-1"})
	b.expect(401, "GET", "/api/auth/me", nil)
	a.expect(200, "GET", "/api/auth/me", nil)
	e.client("").expect(401, "POST", "/api/auth/login", M{"email": email, "password": "password-123"})
	e.client("").expect(200, "POST", "/api/auth/login", M{"email": email, "password": "new-password-1"})
}

func TestAdminSuspendBlocksLoginAndEndsSessions(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	created := must[domain.User](admin, 201, "POST", "/api/admin/users", M{"name": "Citra", "email": "citra@test.local", "password": "password-123"})
	admin.expect(409, "POST", "/api/admin/users", M{"name": "Citra", "email": "CITRA@test.local", "password": "password-123"})

	u := e.client("")
	u.expect(200, "POST", "/api/auth/login", M{"email": "citra@test.local", "password": "password-123"})
	if cats := must[[]domain.Category](u, 200, "GET", "/api/categories", nil); len(cats) == 0 {
		t.Fatal("admin-created user has no default categories")
	}

	path := fmt.Sprintf("/api/admin/users/%d/status", created.ID)
	admin.expect(400, "POST", path, M{"status": "suspended", "reason": "  "})
	admin.expect(200, "POST", path, M{"status": "suspended", "reason": "Melanggar ketentuan"})
	u.expect(401, "GET", "/api/auth/me", nil)

	again := e.client("")
	body := again.expect(403, "POST", "/api/auth/login", M{"email": "citra@test.local", "password": "password-123"})
	if !strings.Contains(body, "Melanggar ketentuan") {
		t.Fatalf("reason not shown: %s", body)
	}
	// The reason stays hidden from someone who does not know the password.
	if body := e.client("").expect(401, "POST", "/api/auth/login", M{"email": "citra@test.local", "password": "wrong-password"}); strings.Contains(body, "Melanggar") {
		t.Fatalf("reason leaked: %s", body)
	}

	admin.expect(200, "POST", path, M{"status": "active"})
	again.expect(200, "POST", "/api/auth/login", M{"email": "citra@test.local", "password": "password-123"})
}

func TestAdminFreezeMakesAccountReadOnlyAndPausesRoutines(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	c := e.newUser("")
	w := c.wallet("BCA", 100_000)
	cat := c.category(domain.TxExpense, "Makan")
	tx := must[domain.Transaction](c, 201, "POST", "/api/transactions", txBody(w.ID, cat, "expense", 1_000, "2026-01-10"))
	c.expect(201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": cat, "type": "expense", "name": "Rutin",
		"amount": 1_000, "frequency": "daily", "startOn": "2026-01-16",
	})
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	path := fmt.Sprintf("/api/admin/users/%d/status", me.ID)
	admin.expect(200, "POST", path, M{"status": "frozen", "reason": "Verifikasi data"})

	// Every kind of change is refused, on the session that was already open.
	for _, r := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/transactions", txBody(w.ID, cat, "expense", 1, "2026-01-10")},
		{"PUT", fmt.Sprintf("/api/transactions/%d", tx.ID), txBody(w.ID, cat, "expense", 1, "2026-01-10")},
		{"DELETE", fmt.Sprintf("/api/transactions/%d", tx.ID), nil},
		{"POST", "/api/wallets", M{"name": "X", "type": "cash", "color": "c1"}},
		{"DELETE", fmt.Sprintf("/api/wallets/%d", w.ID), nil},
		{"POST", "/api/categories", M{"name": "X", "type": "expense", "color": "c1"}},
		{"POST", "/api/recurring", M{}},
	} {
		if body := c.expect(403, r.method, r.path, r.body); !strings.Contains(body, "ACCOUNT_FROZEN") {
			t.Fatalf("%s %s: %s", r.method, r.path, body)
		}
	}
	// Reading and exporting still work.
	c.expect(200, "GET", "/api/transactions", nil)
	c.expect(200, "GET", "/api/dashboard", nil)
	if s, _ := c.do("GET", "/api/reports/monthly.pdf?month=2026-01", nil); s != 200 {
		t.Fatalf("pdf while frozen: %d", s)
	}
	// Scheduled money movement also stops.
	e.clock.set("2026-01-20")
	if n, err := e.app.Recurring.RunDue(context.Background()); err != nil || n != 0 {
		t.Fatalf("posted %d while frozen (err %v)", n, err)
	}
	if got := c.balance(w.ID); got != 99_000 {
		t.Fatalf("balance %d", got)
	}
	admin.expect(200, "POST", path, M{"status": "active"})
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, cat, "expense", 1, "2026-01-10"))
}

func TestRoleBoundaries(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	u := e.newUser("")
	anon := e.client("")

	u.expect(403, "GET", "/api/admin/users", nil)
	u.expect(403, "GET", "/api/admin/audit-logs", nil)
	anon.expect(401, "GET", "/api/admin/users", nil)
	anon.expect(401, "GET", "/api/wallets", nil)
	admin.expect(403, "GET", "/api/wallets", nil)

	me := must[domain.User](admin, 200, "GET", "/api/auth/me", nil)
	admin.expect(403, "POST", fmt.Sprintf("/api/admin/users/%d/status", me.ID), M{"status": "suspended", "reason": "x"})

	// Registration cannot be used to grant admin rights.
	c := e.client("")
	got := must[domain.User](c, 201, "POST", "/api/auth/register", M{"name": "Z", "email": "z@test.local", "password": "password-123", "role": "admin"})
	if got.Role != domain.RoleUser {
		t.Fatalf("role %s", got.Role)
	}
}

func TestLoginRateLimitAndCrossSiteProtection(t *testing.T) {
	e := newEnv(t)
	c := e.client("")
	body := M{"email": "x@test.local", "password": "wrong-password"}
	for range 8 {
		c.expect(401, "POST", "/api/auth/login", body)
	}
	c.expect(429, "POST", "/api/auth/login", body)

	req, _ := http.NewRequest("POST", e.srv.URL+"/api/auth/login", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatalf("cross-site POST: %d", resp.StatusCode)
	}
}

func TestAuditLogRecordsLocationIPDeviceAndTime(t *testing.T) {
	geo := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"success":true,"city":"Yogyakarta","region":"DI Yogyakarta","country":"Indonesia"}`)
	}))
	defer geo.Close()
	e := newEnv(t, func(p *app.Params) {
		p.GeoIPURL = geo.URL + "/"
		p.TrustProxy = true
	})
	u := e.client("Mozilla/5.0 (iPhone; CPU iPhone OS 17_0 like Mac OS X) AppleWebKit Version/17.0 Mobile Safari/604.1")

	req, _ := http.NewRequest("POST", e.srv.URL+"/api/auth/register",
		strings.NewReader(`{"name":"Dewi","email":"dewi@test.local","password":"password-123"}`))
	req.Header.Set("User-Agent", u.ua)
	req.Header.Set("X-Forwarded-For", "203.0.113.9")
	resp, err := u.hc.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	u.do("POST", "/api/auth/logout", nil)
	e.client("").do("POST", "/api/auth/login", M{"email": "dewi@test.local", "password": "wrong-password"})

	admin := e.admin()
	var entry domain.AuditLog
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) { // The location arrives from a background goroutine.
		p := must[domain.Page[domain.AuditLog]](admin, 200, "GET", "/api/admin/audit-logs?event=register", nil)
		if p.Total == 1 && p.Items[0].Location != "" {
			entry = p.Items[0]
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if entry.Location != "Yogyakarta, DI Yogyakarta, Indonesia" {
		t.Fatalf("location %q", entry.Location)
	}
	if entry.IP != "203.0.113.9" || entry.Device != "Safari di iPhone" || entry.Email != "dewi@test.local" || entry.CreatedAt.IsZero() {
		t.Fatalf("entry %+v", entry)
	}
	failed := must[domain.Page[domain.AuditLog]](admin, 200, "GET", "/api/admin/audit-logs?event=login_failed", nil)
	if failed.Total != 1 || failed.Items[0].Detail != "Kata sandi salah" {
		t.Fatalf("failed logins %+v", failed)
	}
	if p := must[domain.Page[domain.AuditLog]](admin, 200, "GET", "/api/admin/audit-logs?search=203.0.113", nil); p.Total == 0 {
		t.Fatal("search by IP found nothing")
	}
}

func TestSpoofedForwardedHeaderIgnoredByDefault(t *testing.T) {
	e := newEnv(t)
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/auth/register",
		strings.NewReader(`{"name":"Eka","email":"eka@test.local","password":"password-123"}`))
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	p := must[domain.Page[domain.AuditLog]](e.admin(), 200, "GET", "/api/admin/audit-logs?event=register", nil)
	if p.Items[0].IP == "1.2.3.4" {
		t.Fatal("client forged its own audit IP")
	}
}

func TestWalletRemovalKeepsHistory(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	empty := c.wallet("Kosong", 0)
	used := c.wallet("Dipakai", 10_000)
	cat := c.category(domain.TxExpense, "Makan")
	c.expect(201, "POST", "/api/transactions", txBody(used.ID, cat, "expense", 1_000, "2026-01-10"))

	if r := must[map[string]bool](c, 200, "DELETE", fmt.Sprintf("/api/wallets/%d", empty.ID), nil); r["archived"] {
		t.Fatal("unused wallet should be deleted, not archived")
	}
	if r := must[map[string]bool](c, 200, "DELETE", fmt.Sprintf("/api/wallets/%d", used.ID), nil); !r["archived"] {
		t.Fatal("used wallet should be archived")
	}
	if w := must[[]domain.Wallet](c, 200, "GET", "/api/wallets", nil); len(w) != 0 {
		t.Fatalf("still listed: %+v", w)
	}
	if p := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil); p.Total != 1 || p.Items[0].WalletName != "Dipakai" {
		t.Fatalf("history lost: %+v", p)
	}
	c.expect(400, "POST", "/api/transactions", txBody(used.ID, cat, "expense", 1, "2026-01-10"))
	// A category in use cannot be deleted either.
	c.expect(409, "DELETE", fmt.Sprintf("/api/categories/%d", cat), nil)
}
