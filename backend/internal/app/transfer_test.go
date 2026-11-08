package app_test

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"receh/internal/domain"
)

func transferBody(from, to, amount, fee int64, on string) M {
	return M{"fromWalletId": from, "toWalletId": to, "amount": amount, "fee": fee, "note": "Top up", "occurredOn": on}
}

func dashboard(c *client) domain.Dashboard {
	return must[domain.Dashboard](c, 200, "GET", "/api/dashboard?unit=month&offset=0", nil)
}

func sliceTotal(slices []domain.CategorySlice, name string) int64 {
	for _, s := range slices {
		if s.Name == name {
			return s.Total
		}
	}
	return 0
}

func TestTransferMovesMoneyAndBooksFeeAsExpense(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	bca := c.wallet("BCA", 500_000)
	gopay := c.wallet("GoPay", 0)
	before := dashboard(c)

	tr := must[domain.Transfer](c, 201, "POST", "/api/transfers", transferBody(bca.ID, gopay.ID, 100_000, 1_000, "2026-01-15"))
	if tr.FromWalletName != "BCA" || tr.ToWalletName != "GoPay" || tr.Fee != 1_000 {
		t.Fatalf("transfer = %+v", tr)
	}
	// The source pays amount + fee, the destination receives the amount.
	if a, b := c.balance(bca.ID), c.balance(gopay.ID); a != 399_000 || b != 100_000 {
		t.Fatalf("balances %d %d, want 399000 100000", a, b)
	}

	// Only the fee is spending. The move itself is not income or expense.
	d := dashboard(c)
	if d.Totals.Income != before.Totals.Income || d.Totals.Expense != before.Totals.Expense+1_000 {
		t.Fatalf("totals %+v, before %+v", d.Totals, before.Totals)
	}
	if got := sliceTotal(d.ExpenseSlices, "Biaya admin"); got != 1_000 {
		t.Fatalf("fee slice = %d", got)
	}
	if d.TotalBalance != 499_000 {
		t.Fatalf("total balance %d", d.TotalBalance)
	}

	// Across all wallets the transfer is one row, plus its fee expense.
	all := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil)
	if all.Total != 2 {
		t.Fatalf("history has %d rows, want transfer + fee", all.Total)
	}
	var leg, fee *domain.Transaction
	for i := range all.Items {
		switch all.Items[i].Type {
		case domain.TxTransferOut:
			leg = &all.Items[i]
		case domain.TxExpense:
			fee = &all.Items[i]
		}
	}
	if leg == nil || leg.CounterpartName != "GoPay" || leg.TransferFee != 1_000 || leg.Amount != 100_000 {
		t.Fatalf("transfer row = %+v", leg)
	}
	if fee == nil || fee.CategoryName != "Biaya admin" || fee.TransferID == nil || *fee.TransferID != tr.ID {
		t.Fatalf("fee row = %+v", fee)
	}
	// Filtered to the destination, its incoming leg explains its balance.
	in := must[domain.Page[domain.Transaction]](c, 200, "GET", fmt.Sprintf("/api/transactions?walletId=%d", gopay.ID), nil)
	if in.Total != 1 || in.Items[0].Type != domain.TxTransferIn || in.Items[0].CounterpartName != "BCA" {
		t.Fatalf("GoPay history = %+v", in.Items)
	}
	only := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions?type=transfer", nil)
	if only.Total != 1 {
		t.Fatalf("transfer filter = %d rows", only.Total)
	}

	// Rows of a transfer cannot be edited on their own.
	c.expect(409, "PUT", fmt.Sprintf("/api/transactions/%d", fee.ID), txBody(bca.ID, fee.CategoryID, "expense", 1, "2026-01-15"))
	c.expect(409, "DELETE", fmt.Sprintf("/api/transactions/%d", leg.ID), nil)

	// The monthly report lists it and keeps it out of income and expense.
	rep := must[domain.MonthlyReport](c, 200, "GET", "/api/reports/monthly?month=2026-01", nil)
	if rep.Totals.Expense != 1_000 || rep.Totals.Income != 0 {
		t.Fatalf("report totals %+v", rep.Totals)
	}
	c.expect(200, "GET", "/api/reports/monthly.pdf?month=2026-01", nil)
}

func TestTransferWithoutFeeAndValidation(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	a := c.wallet("BCA", 50_000)
	b := c.wallet("BCA Tabungan", 0)

	c.expect(201, "POST", "/api/transfers", transferBody(a.ID, b.ID, 20_000, 0, "2026-01-15"))
	if x, y := c.balance(a.ID), c.balance(b.ID); x != 30_000 || y != 20_000 {
		t.Fatalf("balances %d %d", x, y)
	}
	all := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil)
	if all.Total != 1 {
		t.Fatalf("a free transfer adds no fee row, got %d rows", all.Total)
	}

	// The fee counts against the balance: 29.000 + 2.500 > 30.000.
	body := c.expect(409, "POST", "/api/transfers", transferBody(a.ID, b.ID, 29_000, 2_500, "2026-01-15"))
	if !strings.Contains(body, "Rp 31.500") || !strings.Contains(body, "Rp 2.500") || !strings.Contains(body, "Rp 30.000") {
		t.Fatalf("shortfall message should show need, fee and balance: %s", body)
	}
	c.expect(201, "POST", "/api/transfers", transferBody(a.ID, b.ID, 27_500, 2_500, "2026-01-15"))
	if got := c.balance(a.ID); got != 0 {
		t.Fatalf("exact amount + fee should empty the wallet, got %d", got)
	}

	for name, in := range map[string]M{
		"same wallet":  transferBody(a.ID, a.ID, 1, 0, "2026-01-15"),
		"zero amount":  transferBody(b.ID, a.ID, 0, 0, "2026-01-15"),
		"negative fee": transferBody(b.ID, a.ID, 1, -1, "2026-01-15"),
		"huge fee":     transferBody(b.ID, a.ID, 1, 100_001, "2026-01-15"),
		"future":       transferBody(b.ID, a.ID, 1, 0, "2026-03-01"),
		"no wallet":    transferBody(b.ID, 999_999, 1, 0, "2026-01-15"),
	} {
		if status, data := c.do("POST", "/api/transfers", in); status != 400 {
			t.Errorf("%s: status %d: %s", name, status, data)
		}
	}

	// Another user's wallet is invisible.
	other := e.newUser("")
	theirs := other.wallet("Punya orang", 10_000)
	c.expect(400, "POST", "/api/transfers", transferBody(theirs.ID, b.ID, 1_000, 0, "2026-01-15"))
	c.expect(400, "POST", "/api/transfers", transferBody(b.ID, theirs.ID, 1_000, 0, "2026-01-15"))
	if got := other.balance(theirs.ID); got != 10_000 {
		t.Fatalf("other user's balance changed: %d", got)
	}
}

func TestTransferEditAndDeleteReverseExactly(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	bca := c.wallet("BCA", 200_000)
	gopay := c.wallet("GoPay", 0)
	dana := c.wallet("DANA", 0)

	tr := must[domain.Transfer](c, 201, "POST", "/api/transfers", transferBody(bca.ID, gopay.ID, 100_000, 1_000, "2026-01-15"))
	// Change destination, amount and fee in one edit.
	upd := must[domain.Transfer](c, 200, "PUT", fmt.Sprintf("/api/transfers/%d", tr.ID), transferBody(bca.ID, dana.ID, 50_000, 2_500, "2026-01-14"))
	if upd.ToWalletName != "DANA" || upd.Fee != 2_500 || upd.OccurredOn != "2026-01-14" {
		t.Fatalf("updated = %+v", upd)
	}
	if a, g, d := c.balance(bca.ID), c.balance(gopay.ID), c.balance(dana.ID); a != 147_500 || g != 0 || d != 50_000 {
		t.Fatalf("after edit %d %d %d", a, g, d)
	}
	if fee := sliceTotal(dashboard(c).ExpenseSlices, "Biaya admin"); fee != 2_500 {
		t.Fatalf("fee after edit = %d", fee)
	}
	// Removing the fee removes its expense row.
	c.expect(200, "PUT", fmt.Sprintf("/api/transfers/%d", tr.ID), transferBody(bca.ID, dana.ID, 50_000, 0, "2026-01-14"))
	if all := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil); all.Total != 1 {
		t.Fatalf("rows after dropping fee: %d", all.Total)
	}

	// Money already spent at the destination cannot be pulled back.
	makan := c.category(domain.TxExpense, "Makan")
	spent := must[domain.Transaction](c, 201, "POST", "/api/transactions", txBody(dana.ID, makan, "expense", 40_000, "2026-01-15"))
	c.expect(409, "DELETE", fmt.Sprintf("/api/transfers/%d", tr.ID), nil)
	c.expect(409, "PUT", fmt.Sprintf("/api/transfers/%d", tr.ID), transferBody(bca.ID, dana.ID, 20_000, 0, "2026-01-14"))
	c.expect(204, "DELETE", fmt.Sprintf("/api/transactions/%d", spent.ID), nil)

	c.expect(204, "DELETE", fmt.Sprintf("/api/transfers/%d", tr.ID), nil)
	if a, d := c.balance(bca.ID), c.balance(dana.ID); a != 200_000 || d != 0 {
		t.Fatalf("after delete %d %d", a, d)
	}
	if all := must[domain.Page[domain.Transaction]](c, 200, "GET", "/api/transactions", nil); all.Total != 0 {
		t.Fatalf("rows left after delete: %d", all.Total)
	}
	c.expect(404, "GET", fmt.Sprintf("/api/transfers/%d", tr.ID), nil)
}

func TestTransferRemembersFeePerRoute(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	bca := c.wallet("BCA", 1_000_000)
	gopay := c.wallet("GoPay", 0)
	bri := c.wallet("BRI", 0)

	type hint struct {
		Fee   int64 `json:"fee"`
		Found bool  `json:"found"`
	}
	route := func(from, to int64) hint {
		return must[hint](c, 200, "GET", fmt.Sprintf("/api/transfers/last-fee?from=%d&to=%d", from, to), nil)
	}
	if h := route(bca.ID, gopay.ID); h.Found {
		t.Fatalf("no history yet, got %+v", h)
	}
	c.expect(201, "POST", "/api/transfers", transferBody(bca.ID, gopay.ID, 50_000, 1_000, "2026-01-15"))
	c.expect(201, "POST", "/api/transfers", transferBody(bca.ID, bri.ID, 50_000, 2_500, "2026-01-15"))
	if h := route(bca.ID, gopay.ID); !h.Found || h.Fee != 1_000 {
		t.Fatalf("BCA→GoPay hint %+v", h)
	}
	if h := route(bca.ID, bri.ID); !h.Found || h.Fee != 2_500 {
		t.Fatalf("BCA→BRI hint %+v", h)
	}
	if h := route(gopay.ID, bca.ID); h.Found {
		t.Fatalf("the reverse route has its own fee, got %+v", h)
	}
}

// Transfers in both directions at once must neither deadlock (InnoDB or the
// wallet mutexes) nor lose a rupiah: the only money that leaves the system is
// the fees.
func TestConcurrentTransfersBothWaysStayConsistent(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	a := c.wallet("A", 1_000_000)
	b := c.wallet("B", 1_000_000)

	const each = 50
	var wg sync.WaitGroup
	var ok, refused atomic.Int64
	for i := range 2 * each {
		wg.Add(1)
		go func() {
			defer wg.Done()
			from, to := a.ID, b.ID
			if i%2 == 1 {
				from, to = b.ID, a.ID
			}
			status, data := c.do("POST", "/api/transfers", transferBody(from, to, 10_000, 1_000, "2026-01-15"))
			switch status {
			case 201:
				ok.Add(1)
			case 409:
				refused.Add(1)
			default:
				t.Errorf("status %d: %s", status, data)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 2*each {
		t.Fatalf("%d succeeded, %d refused; every transfer was affordable", ok.Load(), refused.Load())
	}
	total := c.balance(a.ID) + c.balance(b.ID)
	if want := int64(2_000_000 - 2*each*1_000); total != want {
		t.Fatalf("total %d, want %d (start minus fees)", total, want)
	}
	if a, b := c.balance(a.ID), c.balance(b.ID); a != b {
		t.Fatalf("symmetric traffic should leave equal balances: %d %d", a, b)
	}
}

// Draining one wallet with parallel transfers stops at exactly zero.
func TestConcurrentTransfersNeverOverdraw(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	src := c.wallet("BCA", 55_000)
	dst := c.wallet("GoPay", 0)

	var wg sync.WaitGroup
	var ok atomic.Int64
	for range 40 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if status, _ := c.do("POST", "/api/transfers", transferBody(src.ID, dst.ID, 10_000, 1_000, "2026-01-15")); status == 201 {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 5 {
		t.Fatalf("%d transfers succeeded, want 5 (5 × 11.000 = 55.000)", ok.Load())
	}
	if s, d := c.balance(src.ID), c.balance(dst.ID); s != 0 || d != 50_000 {
		t.Fatalf("balances %d %d", s, d)
	}
}

func TestFrozenAccountCannotTransfer(t *testing.T) {
	e := newEnv(t)
	admin := e.admin()
	c := e.newUser("")
	a := c.wallet("A", 10_000)
	b := c.wallet("B", 0)
	tr := must[domain.Transfer](c, 201, "POST", "/api/transfers", transferBody(a.ID, b.ID, 1_000, 0, "2026-01-15"))
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	admin.expect(200, "POST", fmt.Sprintf("/api/admin/users/%d/status", me.ID), M{"status": "frozen", "reason": "Cek"})

	c.expect(403, "POST", "/api/transfers", transferBody(a.ID, b.ID, 1_000, 0, "2026-01-15"))
	c.expect(403, "PUT", fmt.Sprintf("/api/transfers/%d", tr.ID), transferBody(a.ID, b.ID, 2_000, 0, "2026-01-15"))
	c.expect(403, "DELETE", fmt.Sprintf("/api/transfers/%d", tr.ID), nil)
	c.expect(200, "GET", fmt.Sprintf("/api/transfers/%d", tr.ID), nil)
}

// An old account (created before the fee category existed) gets it on its
// first transfer with a fee, and a deleted one comes back.
func TestFeeCategoryIsCreatedWhenMissing(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	a := c.wallet("A", 10_000)
	b := c.wallet("B", 0)
	c.expect(204, "DELETE", fmt.Sprintf("/api/categories/%d", c.category(domain.TxExpense, "Biaya admin")), nil)

	c.expect(201, "POST", "/api/transfers", transferBody(a.ID, b.ID, 1_000, 500, "2026-01-15"))
	c.category(domain.TxExpense, "Biaya admin") // Fails the test if absent.
}

// The dashboard is cached, but never past a change: each write bumps the
// user's version, so the next read is rebuilt.
func TestDashboardCacheNeverServesStaleData(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 100_000)
	makan := c.category(domain.TxExpense, "Makan")

	if d := dashboard(c); d.Totals.Expense != 0 || d.TotalBalance != 100_000 {
		t.Fatalf("start %+v", d.Totals)
	}
	dashboard(c) // Second read is served from cache.
	tx := must[domain.Transaction](c, 201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 25_000, "2026-01-15"))
	if d := dashboard(c); d.Totals.Expense != 25_000 || d.TotalBalance != 75_000 || len(d.Latest) != 1 {
		t.Fatalf("after expense: totals %+v balance %d latest %d", d.Totals, d.TotalBalance, len(d.Latest))
	}
	c.expect(204, "DELETE", fmt.Sprintf("/api/transactions/%d", tx.ID), nil)
	if d := dashboard(c); d.Totals.Expense != 0 || d.TotalBalance != 100_000 {
		t.Fatalf("after delete %+v", d.Totals)
	}
	// Renaming a wallet shows up too.
	c.expect(200, "PUT", fmt.Sprintf("/api/wallets/%d", w.ID), M{"name": "BCA Utama", "type": "bank", "color": "c2"})
	if d := dashboard(c); d.Wallets[0].Name != "BCA Utama" {
		t.Fatalf("wallet name %q", d.Wallets[0].Name)
	}
	// A posting by the background scheduler invalidates as well.
	c.expect(201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": makan, "type": "expense", "name": "Kos",
		"amount": 10_000, "frequency": "daily", "startOn": "2026-01-16",
	})
	dashboard(c)
	e.clock.set("2026-01-16")
	if _, err := e.app.Recurring.RunDue(t.Context()); err != nil {
		t.Fatal(err)
	}
	if d := dashboard(c); d.TotalBalance != 90_000 {
		t.Fatalf("after scheduler: balance %d", d.TotalBalance)
	}

	// Each user's cache is their own.
	other := e.newUser("")
	if d := dashboard(other); d.TotalBalance != 0 || len(d.Wallets) != 0 {
		t.Fatalf("other user sees %+v", d.Wallets)
	}
}
