package app_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"receh/internal/domain"
)

type inbox struct {
	Items  []domain.Notification `json:"items"`
	Unread int                   `json:"unread"`
}

func inboxOf(c *client) inbox { return must[inbox](c, 200, "GET", "/api/notifications", nil) }

func kinds(in inbox) map[domain.NotificationKind]int {
	out := map[domain.NotificationKind]int{}
	for _, n := range in.Items {
		out[n.Kind]++
	}
	return out
}

func sweep(t *testing.T, e *env) int {
	t.Helper()
	n, err := e.app.Notifications.Sweep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// eventually polls until cond holds; the worker runs in its own goroutine.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestBudgetNotificationsArriveRightAfterSpendingAndOnlyOnce(t *testing.T) {
	e := newEnv(t)
	go e.app.Notifications.RunWorker(t.Context()) // the event-driven path
	c := e.newUser("")
	w := c.wallet("BCA", 1_000_000)
	makan := c.category(domain.TxExpense, "Makan")
	c.expect(201, "POST", "/api/budgets", M{"categoryId": makan, "amount": 100_000})

	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 85_000, "2026-01-15"))
	eventually(t, "budget warning", func() bool { return kinds(inboxOf(c))[domain.NotifyBudgetWarning] == 1 })
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 20_000, "2026-01-15"))
	eventually(t, "budget over", func() bool { return kinds(inboxOf(c))[domain.NotifyBudgetOver] == 1 })

	// Sweeping again, and more spending, never repeats the same notice.
	sweep(t, e)
	sweep(t, e)
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 1_000, "2026-01-15"))
	time.Sleep(200 * time.Millisecond)
	in := inboxOf(c)
	if k := kinds(in); k[domain.NotifyBudgetWarning] != 1 || k[domain.NotifyBudgetOver] != 1 {
		t.Fatalf("duplicates: %v", k)
	}
	over := in.Items[0]
	if over.Kind != domain.NotifyBudgetOver || !strings.Contains(over.Title, "Makan") || !strings.Contains(over.Body, "Rp 105.000") || over.Link != "/app/anggaran" {
		t.Fatalf("over notice %+v", over)
	}

	// Read state.
	if in.Unread != 2 {
		t.Fatalf("unread %d", in.Unread)
	}
	c.expect(204, "POST", "/api/notifications/read", M{"ids": []int64{over.ID}})
	if n := must[map[string]int](c, 200, "GET", "/api/notifications/unread", nil)["unread"]; n != 1 {
		t.Fatalf("unread after one: %d", n)
	}
	// Someone else cannot mark or see them.
	other := e.newUser("")
	other.expect(204, "POST", "/api/notifications/read", M{"all": true})
	if len(inboxOf(other).Items) != 0 || inboxOf(c).Unread != 1 {
		t.Fatal("notifications crossed users")
	}
	c.expect(204, "POST", "/api/notifications/read", M{"all": true})
	if inboxOf(c).Unread != 0 {
		t.Fatal("mark all read")
	}
}

func TestTimeDrivenReminders(t *testing.T) { // 2026-01-15 is a Thursday
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 1_000_000)
	langganan := c.category(domain.TxExpense, "Langganan")
	c.expect(201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": langganan, "type": "expense", "name": "Netflix",
		"amount": 54_000, "frequency": "monthly", "startOn": "2026-01-16",
	})
	busy := e.newUser("")
	bw := busy.wallet("GoPay", 100_000)
	busy.expect(201, "POST", "/api/transactions", txBody(bw.ID, busy.category(domain.TxExpense, "Makan"), "expense", 10_000, "2026-01-15"))

	// Morning: bill due tomorrow, but no evening reminder yet.
	sweep(t, e)
	k := kinds(inboxOf(c))
	if k[domain.NotifyBillDue] != 1 || k[domain.NotifyDailyReminder] != 0 {
		t.Fatalf("morning %v", k)
	}
	// Evening: nothing recorded today → reminder; the busy user gets none.
	e.clock.setHour(21)
	sweep(t, e)
	if k := kinds(inboxOf(c)); k[domain.NotifyDailyReminder] != 1 {
		t.Fatalf("evening %v", k)
	}
	if k := kinds(inboxOf(busy)); k[domain.NotifyDailyReminder] != 0 {
		t.Fatalf("busy user nagged: %v", k)
	}

	// Monday: last week's numbers, compared with the week before.
	busy.expect(201, "POST", "/api/transactions", txBody(bw.ID, busy.category(domain.TxIncome, "Uang Saku"), "income", 50_000, "2026-01-16"))
	busy.expect(201, "POST", "/api/transactions", txBody(bw.ID, busy.category(domain.TxExpense, "Makan"), "expense", 5_000, "2026-01-08"))
	e.clock.set("2026-01-19")
	e.clock.setHour(9)
	sweep(t, e)
	var weekly *domain.Notification
	for _, n := range inboxOf(busy).Items {
		if n.Kind == domain.NotifyWeeklySummary {
			weekly = &n
		}
	}
	if weekly == nil || !strings.Contains(weekly.Body, "Masuk Rp 50.000, keluar Rp 10.000") ||
		!strings.Contains(weekly.Body, "Terbesar: Makan") || !strings.Contains(weekly.Body, "naik 100%") {
		t.Fatalf("weekly %+v", weekly)
	}
	sweep(t, e)
	if k := kinds(inboxOf(busy)); k[domain.NotifyWeeklySummary] != 1 {
		t.Fatalf("weekly repeated: %v", k)
	}
}

func TestLowBalanceGoalAndFailedBillNotifications(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("GoPay", 100_000)
	empty := c.wallet("Kosong", 0) // never used: no low-balance noise
	_ = empty
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, c.category(domain.TxExpense, "Makan"), "expense", 70_000, "2026-01-15"))

	g := must[domain.GoalProgress](c, 201, "POST", "/api/goals", M{"name": "Sepatu", "targetAmount": 100_000})
	c.expect(201, "POST", fmt.Sprintf("/api/goals/%d/contributions", g.ID), M{"amount": 100_000})

	// A bill larger than the balance fails to post and says so.
	c.expect(201, "POST", "/api/recurring", M{
		"walletId": w.ID, "categoryId": c.category(domain.TxExpense, "Kuliah"), "type": "expense", "name": "UKT",
		"amount": 500_000, "frequency": "monthly", "startOn": "2026-01-15",
	})
	sweep(t, e)
	in := inboxOf(c)
	k := kinds(in)
	if k[domain.NotifyLowBalance] != 1 || k[domain.NotifyGoalAchieved] != 1 || k[domain.NotifyBillFailed] != 1 {
		t.Fatalf("got %v", k)
	}
	for _, n := range in.Items {
		if n.Kind == domain.NotifyLowBalance && !strings.Contains(n.Title, "GoPay") {
			t.Fatalf("low balance for the wrong wallet: %+v", n)
		}
	}

	// Preferences: switch low balance off and lower the threshold.
	p := must[map[string]any](c, 200, "GET", "/api/notifications/prefs", nil)
	if p["lowBalanceThreshold"].(float64) != 50_000 || p["reminderHour"].(float64) != 20 {
		t.Fatalf("default prefs %v", p)
	}
	c.expect(400, "PUT", "/api/notifications/prefs", M{"kinds": M{"nope": false}, "lowBalanceThreshold": 1, "reminderHour": 20})
	c.expect(400, "PUT", "/api/notifications/prefs", M{"kinds": M{}, "lowBalanceThreshold": 1, "reminderHour": 24})
	saved := must[map[string]any](c, 200, "PUT", "/api/notifications/prefs",
		M{"kinds": M{"low_balance": false}, "lowBalanceThreshold": 10_000, "reminderHour": 19})
	if saved["kinds"].(map[string]any)["low_balance"] != false || saved["kinds"].(map[string]any)["budget_over"] != true {
		t.Fatalf("saved %v", saved)
	}
	e.clock.set("2026-01-16")
	sweep(t, e)
	if k := kinds(inboxOf(c)); k[domain.NotifyLowBalance] != 1 || k[domain.NotifyGoalAchieved] != 1 {
		t.Fatalf("after disabling: %v", k)
	}

	// Frozen accounts still read their inbox but cannot change preferences.
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	e.admin().expect(200, "POST", fmt.Sprintf("/api/admin/users/%d/status", me.ID), M{"status": "frozen", "reason": "Cek"})
	c.expect(204, "POST", "/api/notifications/read", M{"all": true})
	c.expect(403, "PUT", "/api/notifications/prefs", M{"kinds": M{}, "lowBalanceThreshold": 1, "reminderHour": 20})
}
