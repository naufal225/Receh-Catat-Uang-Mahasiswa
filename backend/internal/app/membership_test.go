package app_test

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"receh/internal/app"
	"receh/internal/domain"
)

func newB2CEnv(t *testing.T) *env {
	return newEnv(t, func(p *app.Params) { p.BillingEnabled = true })
}

func membershipOf(c *client) domain.Membership {
	return must[domain.Membership](c, 200, "GET", "/api/membership", nil)
}

func walletBody(name string) M {
	return M{"name": name, "type": "ewallet", "color": "c1", "balance": 0}
}

func TestPersonalEditionUnlocksEverything(t *testing.T) {
	e := newEnv(t) // BillingEnabled false: the personal edition.
	c := e.newUser("")
	m := membershipOf(c)
	if m.Plan != domain.PlanPremium || m.BillingEnabled {
		t.Fatalf("membership %+v", m)
	}
	for i := range 6 {
		c.expect(201, "POST", "/api/wallets", walletBody(fmt.Sprintf("Dompet %d", i)))
	}
	body := c.expect(400, "POST", "/api/membership/redeem", M{"code": "APAPUN"})
	if !strings.Contains(body, "tidak diperlukan") {
		t.Fatalf("redeem in personal edition: %s", body)
	}
	users := must[domain.Page[domain.User]](e.admin(), 200, "GET", "/api/admin/users", nil)
	for _, u := range users.Items {
		if u.Role == domain.RoleUser && u.Plan != domain.PlanPremium {
			t.Fatalf("admin list shows %s as %q", u.Email, u.Plan)
		}
	}
}

func TestFreePlanLimitAndAdminComplimentaryPremium(t *testing.T) {
	e := newB2CEnv(t)
	admin := e.admin()
	c := e.newUser("")
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)

	m := membershipOf(c)
	if m.Plan != domain.PlanFree || !m.BillingEnabled || m.Limits.MaxWallets != 3 {
		t.Fatalf("new user membership %+v", m)
	}
	for i := range 3 {
		c.expect(201, "POST", "/api/wallets", walletBody(fmt.Sprintf("W%d", i)))
	}
	body := c.expect(403, "POST", "/api/wallets", walletBody("W3"))
	if !strings.Contains(body, "PLAN_LIMIT") {
		t.Fatalf("4th wallet on free plan: %s", body)
	}
	if u := membershipOf(c).Usage; u.Wallets != 3 {
		t.Fatalf("usage %+v", u)
	}

	// Only admins grant premium.
	c.expect(403, "POST", fmt.Sprintf("/api/admin/users/%d/premium", me.ID), M{"note": "aku"})
	grant := must[domain.Entitlement](admin, 201, "POST", fmt.Sprintf("/api/admin/users/%d/premium", me.ID), M{"note": "Keluarga"})
	if grant.Source != domain.SourceComp || grant.EndsAt != nil {
		t.Fatalf("grant %+v", grant)
	}
	m = membershipOf(c)
	if m.Plan != domain.PlanPremium || m.Source != domain.SourceComp || m.Note != "Keluarga" || m.EndsAt != nil {
		t.Fatalf("after grant %+v", m)
	}
	c.expect(201, "POST", "/api/wallets", walletBody("W3"))
	c.expect(201, "POST", "/api/wallets", walletBody("W4"))

	users := must[domain.Page[domain.User]](admin, 200, "GET", "/api/admin/users?search="+me.Email, nil)
	if len(users.Items) != 1 || users.Items[0].Plan != domain.PlanPremium {
		t.Fatalf("admin list %+v", users.Items)
	}

	// Revoking returns the user to free. Their 5 wallets stay; a 6th is refused.
	admin.expect(204, "DELETE", fmt.Sprintf("/api/admin/premium/%d", grant.ID), nil)
	admin.expect(409, "DELETE", fmt.Sprintf("/api/admin/premium/%d", grant.ID), nil)
	if m := membershipOf(c); m.Plan != domain.PlanFree {
		t.Fatalf("after revoke %+v", m)
	}
	c.expect(403, "POST", "/api/wallets", walletBody("W5"))
	if n := len(must[[]domain.Wallet](c, 200, "GET", "/api/wallets", nil)); n != 5 {
		t.Fatalf("existing wallets after downgrade: %d", n)
	}

	// History keeps the revoked grant; the audit log has both actions.
	hist := must[struct{ Items []domain.Entitlement }](admin, 200, "GET", fmt.Sprintf("/api/admin/users/%d/premium", me.ID), nil)
	if len(hist.Items) != 1 || hist.Items[0].RevokedAt == nil {
		t.Fatalf("history %+v", hist.Items)
	}
	for _, ev := range []string{"premium_granted", "premium_revoked"} {
		if logs := must[domain.Page[domain.AuditLog]](admin, 200, "GET", "/api/admin/audit-logs?event="+ev, nil); logs.Total != 1 {
			t.Fatalf("%s audit rows: %d", ev, logs.Total)
		}
	}
	// Admins cannot be given premium.
	admin.expect(400, "POST", "/api/admin/users/1/premium", M{})
}

func TestComplimentaryPremiumExpires(t *testing.T) {
	e := newB2CEnv(t) // today is 2026-01-15
	c := e.newUser("")
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	admin := e.admin()
	admin.expect(400, "POST", fmt.Sprintf("/api/admin/users/%d/premium", me.ID), M{"until": "2026-01-14"})
	admin.expect(201, "POST", fmt.Sprintf("/api/admin/users/%d/premium", me.ID), M{"until": "2026-01-20"})

	e.clock.set("2026-01-20")
	if m := membershipOf(c); m.Plan != domain.PlanPremium || m.EndsAt == nil {
		t.Fatalf("on the last day: %+v", m)
	}
	e.clock.set("2026-01-21")
	if m := membershipOf(c); m.Plan != domain.PlanFree {
		t.Fatalf("the day after: %+v", m)
	}
}

func TestPromoCodeQuotaHoldsUnderConcurrency(t *testing.T) {
	e := newB2CEnv(t)
	admin := e.admin()
	days := 30
	code := must[domain.PromoCode](admin, 201, "POST", "/api/admin/promo-codes",
		M{"code": "Keluarga-2026", "durationDays": days, "maxUses": 5, "note": "Keluarga"})
	if code.Code != "KELUARGA-2026" || code.MaxUses != 5 || !code.Active {
		t.Fatalf("created %+v", code)
	}
	admin.expect(409, "POST", "/api/admin/promo-codes", M{"code": "keluarga-2026", "maxUses": 1})
	auto := must[domain.PromoCode](admin, 201, "POST", "/api/admin/promo-codes", M{"maxUses": 1})
	if !strings.HasPrefix(auto.Code, "RECEH-") || len(auto.Code) != 14 {
		t.Fatalf("generated code %q", auto.Code)
	}

	// 30 people redeem at the same moment; exactly the quota succeeds.
	const n = 30
	clients := make([]*client, n)
	for i := range n {
		email := fmt.Sprintf("promo%d@test.local", i)
		admin.expect(201, "POST", "/api/admin/users", M{"name": "Saudara", "email": email, "password": "password-123"})
		clients[i] = e.client("")
		clients[i].expect(200, "POST", "/api/auth/login", M{"email": email, "password": "password-123"})
	}
	var wg sync.WaitGroup
	var ok, usedUp atomic.Int64
	for _, c := range clients {
		wg.Add(1)
		go func() {
			defer wg.Done()
			status, body := c.do("POST", "/api/membership/redeem", M{"code": " keluarga-2026 "})
			switch {
			case status == 200:
				ok.Add(1)
			case status == 409 && strings.Contains(string(body), "PROMO_USED_UP"):
				usedUp.Add(1)
			default:
				t.Errorf("redeem: %d %s", status, body)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 5 || usedUp.Load() != n-5 {
		t.Fatalf("%d redeemed, %d refused; want 5 and %d", ok.Load(), usedUp.Load(), n-5)
	}
	codes := must[struct{ Items []domain.PromoCode }](admin, 200, "GET", "/api/admin/promo-codes", nil)
	for _, p := range codes.Items {
		if p.Code == "KELUARGA-2026" && p.Used != 5 {
			t.Fatalf("used counter %d", p.Used)
		}
	}
	if logs := must[domain.Page[domain.AuditLog]](admin, 200, "GET", "/api/admin/audit-logs?event=promo_redeemed", nil); logs.Total != 5 {
		t.Fatalf("promo_redeemed audit rows %d", logs.Total)
	}

	// A winner is premium for 30 days, from the code.
	var winner *client
	for _, c := range clients {
		if m := membershipOf(c); m.Plan == domain.PlanPremium {
			winner = c
			if m.Source != domain.SourceCode || m.EndsAt == nil {
				t.Fatalf("winner membership %+v", m)
			}
			break
		}
	}
	// Same person, same code again: refused even if quota were left.
	admin.expect(204, "POST", fmt.Sprintf("/api/admin/promo-codes/%d/active", code.ID), M{"active": true})
	if status, body := winner.do("POST", "/api/membership/redeem", M{"code": auto.Code}); status != 200 {
		t.Fatalf("winner redeeming another code: %d %s", status, body)
	}
	if status, body := winner.do("POST", "/api/membership/redeem", M{"code": auto.Code}); status != 409 {
		t.Fatalf("second redemption of the same code: %d %s", status, body)
	}
}

func TestPromoCodeValidityAndAbuse(t *testing.T) {
	e := newB2CEnv(t)
	admin := e.admin()
	c := e.newUser("")

	c.expect(403, "POST", "/api/admin/promo-codes", M{"maxUses": 1}) // users cannot mint codes
	off := must[domain.PromoCode](admin, 201, "POST", "/api/admin/promo-codes", M{"code": "MATI-1", "maxUses": 5})
	admin.expect(204, "POST", fmt.Sprintf("/api/admin/promo-codes/%d/active", off.ID), M{"active": false})
	c.expect(400, "POST", "/api/membership/redeem", M{"code": "MATI-1"})

	must[domain.PromoCode](admin, 201, "POST", "/api/admin/promo-codes", M{"code": "SINGKAT-1", "maxUses": 5, "expiresOn": "2026-01-16"})
	e.clock.set("2026-01-17")
	c.expect(400, "POST", "/api/membership/redeem", M{"code": "SINGKAT-1"})
	c.expect(400, "POST", "/api/membership/redeem", M{"code": "TIDAK-ADA"})

	// Guessing is rate limited per account.
	var last int
	for range 12 {
		last, _ = c.do("POST", "/api/membership/redeem", M{"code": "TEBAK-TEBAK"})
	}
	if last != 429 {
		t.Fatalf("12th wrong guess: status %d, want 429", last)
	}
	if m := membershipOf(c); m.Plan != domain.PlanFree {
		t.Fatalf("abuse ended premium-free? %+v", m)
	}
}
