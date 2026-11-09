package app_test

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"receh/internal/domain"
)

func goals(c *client) []domain.GoalProgress {
	return must[[]domain.GoalProgress](c, 200, "GET", "/api/goals", nil)
}

func TestManualGoalDepositsWithdrawalsAndPace(t *testing.T) { // today is 2026-01-15
	e := newEnv(t)
	c := e.newUser("")
	c.expect(400, "POST", "/api/goals", M{"name": "Laptop", "targetAmount": 8_000_000, "deadline": "2026-01-14"})
	g := must[domain.GoalProgress](c, 201, "POST", "/api/goals", M{"name": "Laptop", "targetAmount": 8_000_000, "deadline": "2026-07-14"})
	if !g.ManualSaving || g.Saved != 0 || g.DaysLeft == nil || *g.DaysLeft != 181 {
		t.Fatalf("new goal %+v", g)
	}
	path := fmt.Sprintf("/api/goals/%d/contributions", g.ID)
	g = must[domain.GoalProgress](c, 201, "POST", path, M{"amount": 2_000_000, "note": "Gaji part-time"})
	// 6.000.000 left over 181 days = 26 weeks → 230.770/week (rounded up), 7 months → 857.143/month.
	if g.Saved != 2_000_000 || g.Percent != 25 || g.Remaining != 6_000_000 || g.PerWeek != 230_770 || g.PerMonth != 857_143 {
		t.Fatalf("after deposit %+v", g)
	}
	g = must[domain.GoalProgress](c, 201, "POST", path, M{"amount": -500_000, "note": "Darurat"})
	if g.Saved != 1_500_000 {
		t.Fatalf("after withdrawal %+v", g)
	}
	body := c.expect(400, "POST", path, M{"amount": -2_000_000})
	if !strings.Contains(body, "Rp 1.500.000") {
		t.Fatalf("overdraw message: %s", body)
	}
	c.expect(400, "POST", path, M{"amount": 0})

	list := must[[]domain.GoalContribution](c, 200, "GET", path, nil)
	if len(list) != 2 {
		t.Fatalf("contributions %+v", list)
	}
	// Deleting the deposit would leave the withdrawal unbacked.
	var deposit, withdrawal int64
	for _, x := range list {
		if x.Amount > 0 {
			deposit = x.ID
		} else {
			withdrawal = x.ID
		}
	}
	c.expect(400, "DELETE", fmt.Sprintf("%s/%d", path, deposit), nil)
	c.expect(200, "DELETE", fmt.Sprintf("%s/%d", path, withdrawal), nil)
	c.expect(200, "DELETE", fmt.Sprintf("%s/%d", path, deposit), nil)

	// Reaching the target marks it achieved; the dashboard shows it.
	c.expect(201, "POST", path, M{"amount": 8_000_000})
	if gs := goals(c); len(gs) != 1 || !gs[0].Achieved || gs[0].Percent != 100 || gs[0].Remaining != 0 {
		t.Fatalf("achieved %+v", gs)
	}
	if d := dashboard(c); len(d.Goals) != 1 || !d.Goals[0].Achieved {
		t.Fatalf("dashboard goals %+v", d.Goals)
	}

	// Past the deadline without reaching it is "overdue".
	g2 := must[domain.GoalProgress](c, 201, "POST", "/api/goals", M{"name": "Motor", "targetAmount": 1_000_000, "deadline": "2026-01-20"})
	e.clock.set("2026-01-25")
	for _, x := range goals(c) {
		if x.ID == g2.ID && (!x.Overdue || x.PerWeek != 0) {
			t.Fatalf("overdue goal %+v", x)
		}
	}

	// Other users cannot see or touch it.
	other := e.newUser("")
	if len(goals(other)) != 0 {
		t.Fatal("goals leaked")
	}
	other.expect(404, "POST", path, M{"amount": 1})
	other.expect(404, "DELETE", fmt.Sprintf("/api/goals/%d", g.ID), nil)
}

func TestConcurrentGoalWithdrawalsNeverGoNegative(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	g := must[domain.GoalProgress](c, 201, "POST", "/api/goals", M{"name": "Dana darurat", "targetAmount": 1_000_000})
	path := fmt.Sprintf("/api/goals/%d/contributions", g.ID)
	c.expect(201, "POST", path, M{"amount": 100_000})

	var wg sync.WaitGroup
	var ok atomic.Int64
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if status, _ := c.do("POST", path, M{"amount": -10_000}); status == 201 {
				ok.Add(1)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 10 {
		t.Fatalf("%d withdrawals succeeded, want exactly 10", ok.Load())
	}
	if gs := goals(c); gs[0].Saved != 0 {
		t.Fatalf("saved %d, want 0", gs[0].Saved)
	}
}

func TestWalletGoalFollowsBalance(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	bca := c.wallet("BCA", 3_000_000)
	tabungan := c.wallet("Tabungan Laptop", 500_000)
	g := must[domain.GoalProgress](c, 201, "POST", "/api/goals", M{"name": "Laptop", "targetAmount": 2_000_000, "walletId": tabungan.ID})
	if g.ManualSaving || g.Saved != 500_000 || g.WalletName != "Tabungan Laptop" {
		t.Fatalf("wallet goal %+v", g)
	}
	// Saving = moving money to the wallet.
	c.expect(201, "POST", "/api/transfers", transferBody(bca.ID, tabungan.ID, 1_000_000, 0, "2026-01-15"))
	if gs := goals(c); gs[0].Saved != 1_500_000 || gs[0].Percent != 75 {
		t.Fatalf("after transfer %+v", gs[0])
	}
	// Manual contributions do not apply to a wallet goal.
	c.expect(400, "POST", fmt.Sprintf("/api/goals/%d/contributions", g.ID), M{"amount": 1})

	// A manual goal with deposits cannot silently switch to a wallet.
	m := must[domain.GoalProgress](c, 201, "POST", "/api/goals", M{"name": "Liburan", "targetAmount": 1_000_000})
	c.expect(201, "POST", fmt.Sprintf("/api/goals/%d/contributions", m.ID), M{"amount": 50_000})
	c.expect(400, "PUT", fmt.Sprintf("/api/goals/%d", m.ID), M{"name": "Liburan", "targetAmount": 1_000_000, "walletId": tabungan.ID})
	upd := must[domain.GoalProgress](c, 200, "PUT", fmt.Sprintf("/api/goals/%d", m.ID), M{"name": "Liburan Bali", "targetAmount": 2_000_000, "color": "c5"})
	if upd.Name != "Liburan Bali" || upd.Saved != 50_000 || upd.Color != "c5" {
		t.Fatalf("updated %+v", upd)
	}
	// Another user's wallet cannot be linked.
	other := e.newUser("")
	other.expect(400, "POST", "/api/goals", M{"name": "X", "targetAmount": 1, "walletId": tabungan.ID})
}

func TestGoalLimitOnFreePlanAndArchiving(t *testing.T) {
	e := newB2CEnv(t)
	c := e.newUser("")
	first := must[domain.GoalProgress](c, 201, "POST", "/api/goals", M{"name": "Laptop", "targetAmount": 1_000_000})
	body := c.expect(403, "POST", "/api/goals", M{"name": "Motor", "targetAmount": 1_000_000})
	if !strings.Contains(body, "PLAN_LIMIT") {
		t.Fatalf("2nd goal on free plan: %s", body)
	}
	// Archiving a finished goal frees the slot; restoring it is limited again.
	c.expect(200, "POST", fmt.Sprintf("/api/goals/%d/archive", first.ID), M{"archived": true})
	c.expect(201, "POST", "/api/goals", M{"name": "Motor", "targetAmount": 1_000_000})
	c.expect(403, "POST", fmt.Sprintf("/api/goals/%d/archive", first.ID), M{"archived": false})
	if n := len(must[[]domain.GoalProgress](c, 200, "GET", "/api/goals?archived=1", nil)); n != 2 {
		t.Fatalf("with archived: %d", n)
	}
	if u := membershipOf(c).Usage; u.Goals != 1 {
		t.Fatalf("usage %+v", u)
	}
}
