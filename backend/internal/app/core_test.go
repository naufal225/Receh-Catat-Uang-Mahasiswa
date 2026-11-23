package app_test

import (
	"testing"

	"receh/internal/app"
	"receh/internal/domain"
	"receh/internal/modules"
)

// The core must run on its own: a fresh database with no module tables, no
// module routes, nothing registered. This is what "delete modules/finance"
// leaves behind.
func TestCoreRunsWithoutModules(t *testing.T) {
	e := newEnv(t, func(p *app.Params) {
		p.Modules = []modules.Module{}
		p.BillingEnabled = true
	})
	c := e.client("")
	c.expect(201, "POST", "/api/auth/register", M{"name": "Tanpa Modul", "email": "core@test.local", "password": "rahasia-123"})
	if me := must[domain.User](c, 200, "GET", "/api/auth/me", nil); me.Email != "core@test.local" {
		t.Fatalf("me %+v", me)
	}

	m := must[domain.Membership](c, 200, "GET", "/api/membership", nil)
	if m.Plan != domain.PlanFree || len(m.Defs) != 0 || len(m.Limits) != 0 {
		t.Fatalf("membership without modules %+v", m)
	}
	p := must[prefsView](c, 200, "GET", "/api/notifications/prefs", nil)
	if len(p.Kinds) != 0 || len(p.Settings) != 0 {
		t.Fatalf("prefs without modules %+v", p)
	}
	if n, err := e.app.Notifications.Sweep(t.Context()); err != nil || n != 0 {
		t.Fatalf("sweep: %d %v", n, err)
	}

	// Finance routes simply do not exist.
	c.expect(404, "GET", "/api/wallets", nil)
	c.expect(404, "GET", "/api/dashboard", nil)

	// Admin still works: create a user, which runs no module hooks.
	e.admin().expect(201, "POST", "/api/admin/users", M{"name": "Dibuat Admin", "email": "made@test.local", "password": "rahasia-123"})
}
