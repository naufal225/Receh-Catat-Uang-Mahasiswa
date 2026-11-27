package app_test

import (
	"bytes"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"receh/internal/app"
	"receh/internal/domain"
	"receh/internal/platform/payment/midtrans"
	mockpay "receh/internal/platform/payment/mock"
	"receh/internal/platform/payment/xendit"
	"receh/internal/usecase"
)

// payEnv is a b2c app wired to the mock gateway.
type payEnv struct {
	*env
	gw *mockpay.Provider
}

func newPayEnv(t *testing.T) *payEnv {
	gw := mockpay.New("http://pay.local")
	e := newEnv(t, func(p *app.Params) {
		p.BillingEnabled, p.Payments, p.PublicURL = true, gw, "http://pay.local"
	})
	return &payEnv{env: e, gw: gw}
}

func refOf(t *testing.T, p domain.Payment) string {
	t.Helper()
	u, err := url.Parse(p.CheckoutURL)
	if err != nil || u.Query().Get("ref") == "" {
		t.Fatalf("checkout url %q has no ref", p.CheckoutURL)
	}
	return u.Query().Get("ref")
}

func (c *client) checkout(price string) domain.Payment {
	return must[domain.Payment](c, 201, "POST", "/api/billing/checkout", M{"priceId": price})
}

// payMock clicks "pay" on the fake checkout page.
func (c *client) payMock(p domain.Payment) {
	c.e.t.Helper()
	c.expect(204, "POST", "/api/billing/mock/"+refOf(c.e.t, p)+"/paid", nil)
}

func overviewOf(c *client) usecase.BillingOverview {
	return must[usecase.BillingOverview](c, 200, "GET", "/api/billing", nil)
}

func day(t interface{ Format(string) string }) string { return t.Format("2006-01-02") }

func countRows(t *testing.T, e *env, q string, args ...any) int {
	t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestPayingMakesAWorkspaceProAndKeepsOthersFree(t *testing.T) {
	e := newPayEnv(t)
	owner, member := e.person("Ayu"), e.person("Budi")
	team := newTeam(owner.client, "Kos Mawar")
	tok, _ := invite(owner.client, team.ID, member.email, "member")
	join(member, tok)
	ot, mt := owner.in(team.ID), member.in(team.ID)

	if m := membershipOf(ot); m.Plan != domain.PlanFree || m.Limits["finance.wallets"] != 3 {
		t.Fatalf("team starts free: %+v", m)
	}
	// Members see the subscription but not the money, and cannot pay.
	body := mt.expect(403, "POST", "/api/billing/checkout", M{"priceId": "pro_monthly"})
	if !strings.Contains(body, "OWNER_ONLY") {
		t.Fatalf("member checkout: %s", body)
	}

	p := ot.checkout("pro_monthly")
	if p.Status != domain.PaymentPending || p.Amount != 19000 || p.Currency != "IDR" || p.Provider != "mock" {
		t.Fatalf("payment %+v", p)
	}
	if ov := overviewOf(ot); ov.Pending == nil || ov.Pending.ID != p.ID || !ov.IsOwner || ov.Subscription != nil {
		t.Fatalf("overview before paying %+v", ov)
	}
	ot.payMock(p)

	m := membershipOf(ot)
	if m.Plan != domain.PlanPremium || m.Source != domain.SourcePaid || m.Limits["finance.wallets"] != domain.Unlimited {
		t.Fatalf("after paying: %+v", m)
	}
	for i := range 4 { // The free limit is 3.
		ot.expect(201, "POST", "/api/wallets", walletBody(fmt.Sprintf("T%d", i)))
	}
	// A member of the paid workspace gets Pro there; the owner's personal one stays free.
	if m := membershipOf(mt); m.Plan != domain.PlanPremium {
		t.Fatalf("member of paid workspace: %+v", m)
	}
	if m := membershipOf(owner.client); m.Plan != domain.PlanFree {
		t.Fatalf("owner's personal workspace must stay free: %+v", m)
	}
	if m := membershipOf(member.client); m.Plan != domain.PlanFree {
		t.Fatalf("member's own workspace must stay free: %+v", m)
	}

	ov := overviewOf(ot)
	if ov.Subscription == nil || ov.Subscription.State != domain.SubActive || day(ov.Subscription.PeriodEnd) != "2026-02-15" ||
		ov.Pending != nil || len(ov.Payments) != 1 || ov.Payments[0].Status != domain.PaymentPaid {
		t.Fatalf("overview after paying %+v", ov)
	}
	mo := overviewOf(mt)
	if mo.IsOwner || mo.Subscription == nil || mo.Subscription.State != domain.SubActive || len(mo.Payments) != 0 || mo.Pending != nil {
		t.Fatalf("member's view must hide payments: %+v", mo)
	}
	// Only the owner may manage it.
	mt.expect(403, "POST", "/api/billing/subscription/stop", nil)
	mt.expect(403, "POST", fmt.Sprintf("/api/billing/payments/%d/sync", p.ID), nil)
}

func TestCheckoutIsReusedAndReplacedByAnotherChoice(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")

	first := a.checkout("pro_monthly")
	again := a.checkout("pro_monthly")
	if again.ID != first.ID || again.CheckoutURL != first.CheckoutURL {
		t.Fatalf("same choice must reuse the checkout: %+v vs %+v", first, again)
	}
	yearly := a.checkout("pro_yearly")
	if yearly.ID == first.ID || yearly.Amount != 190000 {
		t.Fatalf("yearly %+v", yearly)
	}
	ov := overviewOf(a.client)
	if ov.Pending == nil || ov.Pending.ID != yearly.ID {
		t.Fatalf("pending %+v", ov.Pending)
	}
	statuses := map[int64]domain.PaymentStatus{}
	for _, p := range ov.Payments {
		statuses[p.ID] = p.Status
	}
	if statuses[first.ID] != domain.PaymentExpired {
		t.Fatalf("the replaced checkout must be closed: %v", statuses)
	}
	// The closed checkout cannot be paid any more.
	a.expect(400, "POST", "/api/billing/mock/"+refOf(t, first)+"/paid", nil)
	if m := membershipOf(a.client); m.Plan != domain.PlanFree {
		t.Fatalf("nothing was paid: %+v", m)
	}

	a.expect(400, "POST", "/api/billing/checkout", M{"priceId": "gold"})
	// The owner can drop a checkout.
	a.expect(204, "POST", fmt.Sprintf("/api/billing/payments/%d/cancel", yearly.ID), nil)
	if overviewOf(a.client).Pending != nil {
		t.Fatal("cancelled checkout still pending")
	}
	a.expect(409, "POST", fmt.Sprintf("/api/billing/payments/%d/cancel", yearly.ID), nil)
}

func TestPayingEarlyKeepsTheDaysLeftAndYearlyBuysAYear(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	a.payMock(a.checkout("pro_monthly"))
	if got := day(overviewOf(a.client).Subscription.PeriodEnd); got != "2026-02-15" {
		t.Fatalf("first period ends %s", got)
	}
	e.clock.set("2026-02-01")
	a.payMock(a.checkout("pro_monthly"))
	if got := day(overviewOf(a.client).Subscription.PeriodEnd); got != "2026-03-15" {
		t.Fatalf("paying 14 days early must stack: period ends %s", got)
	}
	e.clock.set("2026-02-20")
	a.payMock(a.checkout("pro_yearly"))
	if got := day(overviewOf(a.client).Subscription.PeriodEnd); got != "2027-03-15" {
		t.Fatalf("yearly on top: period ends %s", got)
	}
}

func TestGraceDaysThenBackToFree(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	a.payMock(a.checkout("pro_monthly")) // 2026-01-15 → 2026-02-15, grace to 2026-02-22
	for i := range 4 {
		a.expect(201, "POST", "/api/wallets", walletBody(fmt.Sprintf("W%d", i)))
	}

	e.clock.set("2026-02-14")
	if s := overviewOf(a.client).Subscription.State; s != domain.SubActive {
		t.Fatalf("day before the end: %s", s)
	}
	e.clock.set("2026-02-18")
	ov := overviewOf(a.client)
	if ov.Subscription.State != domain.SubGrace || membershipOf(a.client).Plan != domain.PlanPremium {
		t.Fatalf("in grace the plan stays Pro: %+v / %+v", ov.Subscription, membershipOf(a.client))
	}
	e.clock.set("2026-02-23")
	ov = overviewOf(a.client)
	if ov.Subscription.State != domain.SubExpired || membershipOf(a.client).Plan != domain.PlanFree {
		t.Fatalf("after grace the workspace is free again: %+v", ov.Subscription)
	}
	// The data is kept; only creating more is refused.
	if w := must[[]map[string]any](a.client, 200, "GET", "/api/wallets", nil); len(w) != 4 {
		t.Fatalf("wallets must survive the downgrade: %d", len(w))
	}
	a.expect(403, "POST", "/api/wallets", walletBody("W4"))

	// Renewing brings it back, counted from now because the old period is over.
	a.payMock(a.checkout("pro_monthly"))
	if got := day(overviewOf(a.client).Subscription.PeriodEnd); got != "2026-03-23" {
		t.Fatalf("renewal after expiry: %s", got)
	}
	a.expect(201, "POST", "/api/wallets", walletBody("W4"))
}

func TestPaidAndComplimentaryPremiumCoexist(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	admin := e.admin()
	admin.expect(201, "POST", fmt.Sprintf("/api/admin/users/%d/premium", a.id), M{"until": "2026-01-20", "note": "keluarga"})
	if m := membershipOf(a.client); m.Source != domain.SourceComp {
		t.Fatalf("comp: %+v", m)
	}
	a.payMock(a.checkout("pro_monthly"))
	if m := membershipOf(a.client); m.Source != domain.SourcePaid {
		t.Fatalf("the longer paid period shows: %+v", m)
	}
	// The comp still counts: when the paid period lapses it would carry on.
	e.clock.set("2026-01-18")
	if membershipOf(a.client).Plan != domain.PlanPremium {
		t.Fatal("premium lost")
	}
	// Promo codes still work on a b2c edition with payments.
	code := must[domain.PromoCode](admin, 201, "POST", "/api/admin/promo-codes", M{"code": "KELUARGA1", "maxUses": 1, "durationDays": 400})
	a.expect(200, "POST", "/api/membership/redeem", M{"code": code.Code})
}

func TestOwnershipTransferKeepsThePaidWorkspacePro(t *testing.T) {
	e := newPayEnv(t)
	a, b := e.person("Ayu"), e.person("Budi")
	team := newTeam(a.client, "Kos")
	tok, _ := invite(a.client, team.ID, b.email, "admin")
	join(b, tok)
	a.in(team.ID).payMock(a.in(team.ID).checkout("pro_monthly"))
	a.expect(204, "POST", fmt.Sprintf("/api/orgs/%d/transfer", team.ID), M{"userId": b.id})
	if m := membershipOf(b.in(team.ID)); m.Plan != domain.PlanPremium || !m.IsOwner {
		t.Fatalf("new owner of a paid workspace: %+v", m)
	}
	// The new owner manages billing; the former owner no longer does.
	a.in(team.ID).expect(403, "POST", "/api/billing/subscription/stop", nil)
	b.in(team.ID).expect(204, "POST", "/api/billing/subscription/stop", nil)
}

func TestStopAndResumeRenewal(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	a.expect(409, "POST", "/api/billing/subscription/stop", nil) // Nothing to stop yet.
	a.payMock(a.checkout("pro_monthly"))
	a.expect(204, "POST", "/api/billing/subscription/stop", nil)
	if s := overviewOf(a.client).Subscription; s.State != domain.SubEnding || !s.CancelAtPeriodEnd {
		t.Fatalf("stopped: %+v", s)
	}
	if membershipOf(a.client).Plan != domain.PlanPremium {
		t.Fatal("stopping renewal must not cut the paid period short")
	}
	a.expect(204, "POST", "/api/billing/subscription/resume", nil)
	if s := overviewOf(a.client).Subscription; s.State != domain.SubActive {
		t.Fatalf("resumed: %+v", s)
	}
}

func TestPendingCheckoutExpiresInTheSweep(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	p := a.checkout("pro_monthly")
	ctx := context.Background()

	if err := e.app.Billing.Sweep(ctx); err != nil { // Too fresh to touch.
		t.Fatal(err)
	}
	if overviewOf(a.client).Pending == nil {
		t.Fatal("a fresh checkout must not be swept")
	}
	e.clock.set("2026-01-17") // Past the 24h payable window.
	if err := e.app.Billing.Sweep(ctx); err != nil {
		t.Fatal(err)
	}
	ov := overviewOf(a.client)
	if ov.Pending != nil || ov.Payments[0].Status != domain.PaymentExpired {
		t.Fatalf("after the sweep: %+v", ov)
	}
	a.expect(400, "POST", "/api/billing/mock/"+refOf(t, p)+"/paid", nil)
	// A new checkout can be made afterwards.
	if q := a.checkout("pro_monthly"); q.ID == p.ID {
		t.Fatal("expired checkout was reused")
	}
}

func TestSweepCatchesAPaymentWhoseWebhookWasLost(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	p := a.checkout("pro_monthly")
	// The gateway recorded the payment but we never heard (no webhook).
	if _, err := e.gw.Complete(refOf(t, p), "paid"); err != nil {
		t.Fatal(err)
	}
	if membershipOf(a.client).Plan != domain.PlanFree {
		t.Fatal("nothing should be granted without hearing from the gateway")
	}
	// The owner can ask for a check right away...
	got := must[domain.Payment](a.client, 200, "POST", fmt.Sprintf("/api/billing/payments/%d/sync", p.ID), nil)
	if got.Status != domain.PaymentPaid || membershipOf(a.client).Plan != domain.PlanPremium {
		t.Fatalf("sync: %+v", got)
	}
	// ...or the background sweep does it.
	b := e.person("Budi")
	q := b.checkout("pro_monthly")
	if _, err := e.gw.Complete(refOf(t, q), "paid"); err != nil {
		t.Fatal(err)
	}
	e.clock.setHour(11) // Past the 5-minute settling window.
	if err := e.app.Billing.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if membershipOf(b.client).Plan != domain.PlanPremium {
		t.Fatal("the sweep missed a paid checkout")
	}
}

func webhook(e *env, provider string, body []byte, headers map[string]string) (int, string) {
	req, _ := http.NewRequest("POST", e.srv.URL+"/api/billing/webhook/"+provider, bytes.NewReader(body))
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(data)
}

func TestWebhookIsAppliedOnceEvenWhenDeliveredManyTimesAtOnce(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	p := a.checkout("pro_monthly")
	if _, err := e.gw.Complete(refOf(t, p), "paid"); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id := "same"
			if i%2 == 0 {
				id = fmt.Sprintf("distinct-%d", i)
			}
			body, _ := json.Marshal(M{"eventId": id, "externalId": p.ExternalID})
			if code, msg := webhook(e.env, "mock", body, nil); code != 200 {
				t.Errorf("webhook %d: %d %s", i, code, msg)
			}
		}()
	}
	wg.Wait()
	if n := countRows(t, e.env, `SELECT COUNT(*) FROM entitlements WHERE source = 'paid'`); n != 1 {
		t.Fatalf("%d paid entitlements, want exactly 1", n)
	}
	if got := day(overviewOf(a.client).Subscription.PeriodEnd); got != "2026-02-15" {
		t.Fatalf("a replayed webhook must not extend the period again: %s", got)
	}
	// Replaying it later changes nothing either.
	body, _ := json.Marshal(M{"eventId": "same", "externalId": p.ExternalID})
	webhook(e.env, "mock", body, nil)
	if n := countRows(t, e.env, `SELECT COUNT(*) FROM entitlements WHERE source = 'paid'`); n != 1 {
		t.Fatalf("replay created another entitlement: %d", n)
	}
}

func TestWebhookEdgeCases(t *testing.T) {
	e := newPayEnv(t)
	body, _ := json.Marshal(M{"eventId": "e1", "externalId": "rc-999-unknown"})
	if code, _ := webhook(e.env, "mock", body, nil); code != 200 {
		t.Fatalf("authentic event for an unknown payment is acknowledged, got %d", code)
	}
	if code, _ := webhook(e.env, "mock", []byte("not json"), nil); code != 500 && code != 400 {
		t.Fatalf("garbage must not be accepted, got %d", code)
	}
	if code, _ := webhook(e.env, "midtrans", body, nil); code != 404 {
		t.Fatalf("a provider that is not configured: %d", code)
	}
	// The mock "pay" endpoint with a made-up reference.
	c := e.client("")
	c.expect(400, "POST", "/api/billing/mock/mock_nope/paid", nil)
	c.expect(400, "POST", "/api/billing/mock/mock_nope/sideways", nil)
}

func TestPersonalEditionHasNoPayments(t *testing.T) {
	e := newEnv(t) // Billing disabled.
	a := e.person("Ayu")
	cat := must[usecase.CatalogView](a.client, 200, "GET", "/api/billing/catalog", nil)
	if cat.Enabled || len(cat.Catalog.Prices) != 0 {
		t.Fatalf("catalog %+v", cat)
	}
	if body := a.expect(400, "POST", "/api/billing/checkout", M{"priceId": "pro_monthly"}); !strings.Contains(body, "tidak diperlukan") {
		t.Fatalf("checkout: %s", body)
	}
	if ov := overviewOf(a.client); ov.Enabled || ov.Subscription != nil {
		t.Fatalf("overview %+v", ov)
	}
	if code, _ := webhook(e, "mock", []byte(`{"eventId":"x","externalId":"y"}`), nil); code != 404 {
		t.Fatalf("webhook on the personal edition: %d", code)
	}
}

func TestCatalogIsPublic(t *testing.T) {
	e := newPayEnv(t)
	resp, err := http.Get(e.srv.URL + "/api/billing/catalog")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var cat usecase.CatalogView
	if err := json.NewDecoder(resp.Body).Decode(&cat); err != nil || resp.StatusCode != 200 {
		t.Fatalf("status %d err %v", resp.StatusCode, err)
	}
	if !cat.Enabled || len(cat.Catalog.Prices) != 2 || cat.Catalog.GraceDays != 7 || cat.Catalog.PlanName["id"] != "Pro" {
		t.Fatalf("catalog %+v", cat)
	}
}

func TestBillingRemindersReachTheOwner(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	a.payMock(a.checkout("pro_monthly")) // ends 2026-02-15
	titles := func() string {
		in := must[struct {
			Items []domain.Notification `json:"items"`
		}](a.client, 200, "GET", "/api/notifications", nil)
		var s []string
		for _, n := range in.Items {
			s = append(s, n.Title)
		}
		return strings.Join(s, "|")
	}
	ctx := context.Background()
	e.clock.set("2026-02-01")
	e.app.Notifications.Sweep(ctx)
	if strings.Contains(titles(), "Langganan Pro") {
		t.Fatalf("too early to remind: %s", titles())
	}
	e.clock.set("2026-02-10")
	e.app.Notifications.Sweep(ctx)
	e.app.Notifications.Sweep(ctx) // Twice: still one reminder.
	if got := titles(); strings.Count(got, "segera berakhir") != 1 {
		t.Fatalf("renewal reminder: %s", got)
	}
	e.clock.set("2026-02-17")
	e.app.Notifications.Sweep(ctx)
	if !strings.Contains(titles(), "sudah berakhir") {
		t.Fatalf("grace reminder: %s", titles())
	}
	e.clock.set("2026-02-25")
	e.app.Notifications.Sweep(ctx)
	if !strings.Contains(titles(), "kembali ke paket Gratis") {
		t.Fatalf("expiry notice: %s", titles())
	}
}

func TestAdminSeesPaymentsAndIntegrationStatus(t *testing.T) {
	e := newPayEnv(t)
	a := e.person("Ayu")
	a.payMock(a.checkout("pro_monthly"))
	admin := e.admin()
	page := must[usecase.PaymentPage](admin, 200, "GET", "/api/admin/payments?status=paid", nil)
	if page.Total != 1 || page.Items[0].PayerEmail != a.email || page.Items[0].OrgName != "Pribadi" {
		t.Fatalf("payments %+v", page)
	}
	if page := must[usecase.PaymentPage](admin, 200, "GET", "/api/admin/payments?status=pending", nil); page.Total != 0 {
		t.Fatalf("pending %+v", page)
	}
	admin.expect(400, "GET", "/api/admin/payments?status=weird", nil)
	st := must[M](admin, 200, "GET", "/api/admin/billing/status", nil)
	if st["enabled"] != true {
		t.Fatalf("status %+v", st)
	}
	a.expect(403, "GET", "/api/admin/payments", nil)
	a.expect(403, "GET", "/api/admin/billing/status", nil)
	// The audit trail names the payment.
	logs := admin.expect(200, "GET", "/api/admin/audit-logs", nil)
	if !strings.Contains(logs, "payment_created") || !strings.Contains(logs, "payment_paid") {
		t.Fatalf("audit log lacks billing events")
	}
}

// ---- Midtrans and Xendit against fake gateways ----------------------------------------

type fakeOrder struct {
	amount int64
	gross  string // what the gateway reports (to test mismatches)
	status string
}

// fakeMidtrans speaks just enough of Midtrans Snap and Core API.
type fakeMidtrans struct {
	t      *testing.T
	key    string
	mu     sync.Mutex
	orders map[string]*fakeOrder
	failOn bool
	srv    *httptest.Server
}

func newFakeMidtrans(t *testing.T) *fakeMidtrans {
	f := &fakeMidtrans{t: t, key: "SB-Mid-server-test", orders: map[string]*fakeOrder{}}
	mux := http.NewServeMux()
	auth := func(w http.ResponseWriter, r *http.Request) bool {
		u, p, ok := r.BasicAuth()
		if !ok || u != f.key || p != "" {
			http.Error(w, `{"error_messages":["Access denied"]}`, 401)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /snap/v1/transactions", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.failOn {
			http.Error(w, `{"error_messages":["boom"]}`, 500)
			return
		}
		var in struct {
			Details struct {
				OrderID string `json:"order_id"`
				Gross   int64  `json:"gross_amount"`
			} `json:"transaction_details"`
		}
		json.NewDecoder(r.Body).Decode(&in)
		f.orders[in.Details.OrderID] = &fakeOrder{amount: in.Details.Gross, gross: fmt.Sprintf("%d.00", in.Details.Gross), status: "none"}
		w.WriteHeader(201)
		fmt.Fprintf(w, `{"token":"tok-%s","redirect_url":"https://pay.example/snap/%s"}`, in.Details.OrderID, in.Details.OrderID)
	})
	mux.HandleFunc("GET /v2/{id}/status", func(w http.ResponseWriter, r *http.Request) {
		if !auth(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		o := f.orders[r.PathValue("id")]
		if o == nil || o.status == "none" {
			fmt.Fprint(w, `{"status_code":"404","status_message":"Transaction doesn't exist."}`)
			return
		}
		fmt.Fprintf(w, `{"status_code":"200","order_id":%q,"gross_amount":%q,"transaction_status":%q,"fraud_status":"accept"}`,
			r.PathValue("id"), o.gross, o.status)
	})
	mux.HandleFunc("POST /v2/{id}/cancel", func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if o := f.orders[r.PathValue("id")]; o != nil && (o.status == "none" || o.status == "pending") {
			o.status = "cancel"
		}
		fmt.Fprint(w, `{"status_code":"200","transaction_status":"cancel"}`)
	})
	f.srv = httptest.NewServer(mux)
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fakeMidtrans) set(order, status string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.orders[order].status = status
}

// notify builds a signed notification body.
func (f *fakeMidtrans) notify(order, status, gross, key string) []byte {
	sum := sha512.Sum512([]byte(order + "200" + gross + key))
	b, _ := json.Marshal(M{"order_id": order, "status_code": "200", "gross_amount": gross, "transaction_status": status,
		"transaction_id": "tx-" + order, "signature_key": hex.EncodeToString(sum[:])})
	return b
}

func newMidtransEnv(t *testing.T) (*env, *fakeMidtrans) {
	f := newFakeMidtrans(t)
	gw := midtrans.New(f.key, false).WithBaseURLs(f.srv.URL, f.srv.URL)
	e := newEnv(t, func(p *app.Params) {
		p.BillingEnabled, p.Payments, p.PublicURL = true, gw, "http://pay.local"
	})
	return e, f
}

func TestMidtransCheckoutWebhookAndTrustButVerify(t *testing.T) {
	e, f := newMidtransEnv(t)
	a := e.person("Ayu")

	p := a.checkout("pro_monthly")
	if p.Provider != "midtrans" || !strings.HasPrefix(p.CheckoutURL, "https://pay.example/snap/rc-") {
		t.Fatalf("payment %+v", p)
	}
	// The customer has not chosen a method yet: Midtrans knows nothing, we stay pending.
	if got := must[domain.Payment](a.client, 200, "POST", fmt.Sprintf("/api/billing/payments/%d/sync", p.ID), nil); got.Status != domain.PaymentPending {
		t.Fatalf("sync before paying: %+v", got)
	}

	// A forged notification (wrong signature) is refused and grants nothing.
	f.set(p.ExternalID, "settlement")
	if code, _ := webhook(e, "midtrans", f.notify(p.ExternalID, "settlement", "19000.00", "wrong-key"), nil); code != 401 {
		t.Fatalf("forged webhook: %d", code)
	}
	if membershipOf(a.client).Plan != domain.PlanFree {
		t.Fatal("a forged webhook granted Pro")
	}

	// A correctly signed notification is only a hint: if the gateway's own
	// answer disagrees on the amount, nothing is granted.
	f.mu.Lock()
	f.orders[p.ExternalID].gross = "1000.00"
	f.mu.Unlock()
	if code, _ := webhook(e, "midtrans", f.notify(p.ExternalID, "settlement", "19000.00", f.key), nil); code != 200 {
		t.Fatalf("signed webhook: %d", code)
	}
	if membershipOf(a.client).Plan != domain.PlanFree {
		t.Fatal("an underpayment granted Pro")
	}
	if logs := e.admin().expect(200, "GET", "/api/admin/audit-logs", nil); !strings.Contains(logs, "payment_mismatch") {
		t.Fatal("the mismatch must be audited")
	}

	// The real settlement arrives.
	f.mu.Lock()
	f.orders[p.ExternalID].gross = "19000.00"
	f.mu.Unlock()
	if code, _ := webhook(e, "midtrans", f.notify(p.ExternalID, "settlement", "19000.00", f.key), nil); code != 200 {
		t.Fatalf("settlement webhook: %d", code)
	}
	if m := membershipOf(a.client); m.Plan != domain.PlanPremium || m.Source != domain.SourcePaid {
		t.Fatalf("after settlement: %+v", m)
	}
	// Delivered again: nothing changes.
	webhook(e, "midtrans", f.notify(p.ExternalID, "settlement", "19000.00", f.key), nil)
	if n := countRows(t, e, `SELECT COUNT(*) FROM entitlements WHERE source = 'paid'`); n != 1 {
		t.Fatalf("%d paid entitlements", n)
	}
}

func TestMidtransLatePaymentOfAnExpiredCheckoutIsStillHonoured(t *testing.T) {
	e, f := newMidtransEnv(t)
	a := e.person("Ayu")
	p := a.checkout("pro_monthly")
	e.clock.set("2026-01-17")
	if err := e.app.Billing.Sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if ov := overviewOf(a.client); ov.Pending != nil || ov.Payments[0].Status != domain.PaymentExpired {
		t.Fatalf("not expired: %+v", ov)
	}
	// Money arrived anyway (the customer paid at the last second).
	f.set(p.ExternalID, "settlement")
	if code, _ := webhook(e, "midtrans", f.notify(p.ExternalID, "settlement", "19000.00", f.key), nil); code != 200 {
		t.Fatalf("late webhook: %d", code)
	}
	if m := membershipOf(a.client); m.Plan != domain.PlanPremium {
		t.Fatalf("a payment that really happened must be honoured: %+v", m)
	}
}

func TestGatewayFailureIsReportedAndRetryable(t *testing.T) {
	e, f := newMidtransEnv(t)
	a := e.person("Ayu")
	f.mu.Lock()
	f.failOn = true
	f.mu.Unlock()
	body := a.expect(502, "POST", "/api/billing/checkout", M{"priceId": "pro_monthly"})
	if !strings.Contains(body, "GATEWAY_ERROR") || strings.Contains(body, "boom") {
		t.Fatalf("gateway error must be generic: %s", body)
	}
	if ov := overviewOf(a.client); ov.Pending != nil {
		t.Fatalf("a checkout that never opened must not stay pending: %+v", ov.Pending)
	}
	f.mu.Lock()
	f.failOn = false
	f.mu.Unlock()
	a.checkout("pro_monthly")
}

// fakeXendit speaks just enough of the Xendit Invoice API.
func TestXenditCallbackNeedsTheTokenAndConfirmsWithTheGateway(t *testing.T) {
	var mu sync.Mutex
	invoices := map[string]map[string]any{} // by id
	mux := http.NewServeMux()
	checkAuth := func(w http.ResponseWriter, r *http.Request) bool {
		u, _, ok := r.BasicAuth()
		if !ok || u != "xnd_test_key" {
			http.Error(w, `{"error_code":"INVALID_API_KEY"}`, 401)
			return false
		}
		return true
	}
	mux.HandleFunc("POST /v2/invoices", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		var in map[string]any
		json.NewDecoder(r.Body).Decode(&in)
		mu.Lock()
		defer mu.Unlock()
		id := fmt.Sprintf("inv-%d", len(invoices)+1)
		invoices[id] = map[string]any{"id": id, "external_id": in["external_id"], "status": "PENDING", "amount": in["amount"],
			"invoice_url": "https://checkout.example/" + id}
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(invoices[id])
	})
	mux.HandleFunc("GET /v2/invoices/{id}", func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(w, r) {
			return
		}
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(invoices[r.PathValue("id")])
	})
	mux.HandleFunc("POST /invoices/{id}/expire!", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if inv := invoices[r.PathValue("id")]; inv != nil && inv["status"] == "PENDING" {
			inv["status"] = "EXPIRED"
		}
		w.Write([]byte(`{}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	e := newEnv(t, func(p *app.Params) {
		p.BillingEnabled, p.PublicURL = true, "http://pay.local"
		p.Payments = xendit.New("xnd_test_key", "cb-token").WithBaseURL(srv.URL)
	})
	a := e.person("Ayu")
	p := a.checkout("pro_yearly")
	if p.Provider != "xendit" || !strings.HasPrefix(p.CheckoutURL, "https://checkout.example/inv-") {
		t.Fatalf("payment %+v", p)
	}
	callback := func(token string, status string) int {
		body, _ := json.Marshal(M{"id": "inv-1", "external_id": p.ExternalID, "status": status, "amount": 190000})
		code, _ := webhook(e, "xendit", body, map[string]string{"x-callback-token": token})
		return code
	}
	mu.Lock()
	invoices["inv-1"]["status"] = "PAID"
	mu.Unlock()
	if code := callback("not-the-token", "PAID"); code != 401 {
		t.Fatalf("wrong token: %d", code)
	}
	if code := callback("", "PAID"); code != 401 {
		t.Fatalf("missing token: %d", code)
	}
	if membershipOf(a.client).Plan != domain.PlanFree {
		t.Fatal("a callback without the token granted Pro")
	}
	if code := callback("cb-token", "PAID"); code != 200 {
		t.Fatalf("callback: %d", code)
	}
	ov := overviewOf(a.client)
	if ov.Subscription == nil || day(ov.Subscription.PeriodEnd) != "2027-01-15" {
		t.Fatalf("a yearly payment: %+v", ov.Subscription)
	}
}
