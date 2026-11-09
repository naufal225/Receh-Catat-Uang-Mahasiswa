package app_test

import (
	"fmt"
	"strings"
	"testing"

	"receh/internal/domain"
)

func budgetMonth(c *client, month string) domain.BudgetMonth {
	path := "/api/budgets"
	if month != "" {
		path += "?month=" + month
	}
	return must[domain.BudgetMonth](c, 200, "GET", path, nil)
}

func findBudget(m domain.BudgetMonth, category string) *domain.BudgetStatus {
	for i := range m.Items {
		if m.Items[i].CategoryName == category {
			return &m.Items[i]
		}
	}
	return nil
}

func TestBudgetsTrackSpendingPerMonth(t *testing.T) { // today is 2026-01-15
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 2_000_000)
	makan := c.category(domain.TxExpense, "Makan")
	minum := c.category(domain.TxExpense, "Minum")

	b := must[domain.Budget](c, 201, "POST", "/api/budgets", M{"categoryId": makan, "amount": 500_000})
	must[domain.Budget](c, 201, "POST", "/api/budgets", M{"categoryId": minum, "amount": 100_000})
	c.expect(409, "POST", "/api/budgets", M{"categoryId": makan, "amount": 1}) // one per category
	c.expect(400, "POST", "/api/budgets", M{"categoryId": c.category(domain.TxIncome, "Beasiswa"), "amount": 1})
	c.expect(400, "POST", "/api/budgets", M{"categoryId": makan + 999, "amount": 1})

	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 300_000, "2026-01-10"))
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 120_000, "2026-01-14"))
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 999_000, "2025-12-31")) // last month
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, minum, "expense", 150_000, "2026-01-12"))
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, c.category(domain.TxExpense, "Hiburan"), "expense", 40_000, "2026-01-12"))

	m := budgetMonth(c, "")
	if m.Month != "2026-01" || !m.IsCurrent || m.DaysLeft != 17 || m.Title != "Januari 2026" {
		t.Fatalf("month header %+v", m)
	}
	mk := findBudget(m, "Makan")
	if mk == nil || mk.Spent != 420_000 || mk.Remaining != 80_000 || mk.Percent != 84 || mk.Level != domain.BudgetWarning {
		t.Fatalf("Makan %+v", mk)
	}
	mn := findBudget(m, "Minum")
	if mn == nil || mn.Spent != 150_000 || mn.Remaining != -50_000 || mn.Level != domain.BudgetOver {
		t.Fatalf("Minum %+v", mn)
	}
	if m.TotalBudget != 600_000 || m.TotalSpent != 570_000 || m.Unbudgeted != 40_000 {
		t.Fatalf("totals %+v", m)
	}

	// Another month is computed from its own transactions.
	dec := budgetMonth(c, "2025-12")
	if d := findBudget(dec, "Makan"); d == nil || d.Spent != 999_000 || dec.IsCurrent {
		t.Fatalf("December %+v", dec)
	}

	// A transfer fee is spending too; it counts against a budget on "Biaya admin".
	fee := c.category(domain.TxExpense, "Biaya admin")
	c.expect(201, "POST", "/api/budgets", M{"categoryId": fee, "amount": 10_000})
	gopay := c.wallet("GoPay", 0)
	c.expect(201, "POST", "/api/transfers", transferBody(w.ID, gopay.ID, 50_000, 2_500, "2026-01-15"))
	if f := findBudget(budgetMonth(c, ""), "Biaya admin"); f == nil || f.Spent != 2_500 {
		t.Fatalf("fee budget %+v", f)
	}

	// Edit and delete.
	upd := must[domain.Budget](c, 200, "PUT", fmt.Sprintf("/api/budgets/%d", b.ID), M{"amount": 400_000})
	if upd.Amount != 400_000 {
		t.Fatalf("updated %+v", upd)
	}
	c.expect(200, "PUT", fmt.Sprintf("/api/budgets/%d", b.ID), M{"amount": 400_000}) // same value is fine
	if mk := findBudget(budgetMonth(c, ""), "Makan"); mk.Level != domain.BudgetOver {
		t.Fatalf("after lowering: %+v", mk)
	}
	c.expect(400, "PUT", fmt.Sprintf("/api/budgets/%d", b.ID), M{"amount": 0})
	c.expect(204, "DELETE", fmt.Sprintf("/api/budgets/%d", b.ID), nil)
	c.expect(404, "DELETE", fmt.Sprintf("/api/budgets/%d", b.ID), nil)

	// Another user sees none of it and cannot touch it.
	other := e.newUser("")
	if len(budgetMonth(other, "").Items) != 0 {
		t.Fatal("budgets leaked to another user")
	}
	other.expect(404, "PUT", fmt.Sprintf("/api/budgets/%d", findBudget(budgetMonth(c, ""), "Minum").ID), M{"amount": 1})
}

func TestBudgetsOnDashboardFollowNewSpending(t *testing.T) {
	e := newEnv(t)
	c := e.newUser("")
	w := c.wallet("BCA", 1_000_000)
	makan := c.category(domain.TxExpense, "Makan")
	c.expect(201, "POST", "/api/budgets", M{"categoryId": makan, "amount": 100_000})

	if d := dashboard(c); len(d.Budgets) != 1 || d.Budgets[0].Spent != 0 {
		t.Fatalf("dashboard budgets %+v", d.Budgets)
	}
	c.expect(201, "POST", "/api/transactions", txBody(w.ID, makan, "expense", 90_000, "2026-01-15"))
	// The dashboard is cached; the new expense must still show at once.
	if d := dashboard(c); d.Budgets[0].Spent != 90_000 || d.Budgets[0].Level != domain.BudgetWarning {
		t.Fatalf("after spending %+v", d.Budgets)
	}
}

func TestBudgetLimitOnFreePlanAndCategoryDeletion(t *testing.T) {
	e := newB2CEnv(t)
	c := e.newUser("")
	names := []string{"Makan", "Minum", "Transportasi", "Listrik"}
	for i, n := range names {
		status, body := c.do("POST", "/api/budgets", M{"categoryId": c.category(domain.TxExpense, n), "amount": 50_000})
		if i < 3 && status != 201 {
			t.Fatalf("budget %d: %d %s", i, status, body)
		}
		if i == 3 && (status != 403 || !strings.Contains(string(body), "PLAN_LIMIT")) {
			t.Fatalf("4th budget on free plan: %d %s", status, body)
		}
	}
	if u := membershipOf(c).Usage; u.Budgets != 3 {
		t.Fatalf("usage %+v", u)
	}

	// Deleting an unused category removes its budget, freeing a slot.
	c.expect(204, "DELETE", fmt.Sprintf("/api/categories/%d", c.category(domain.TxExpense, "Minum")), nil)
	m := budgetMonth(c, "")
	if len(m.Items) != 2 || findBudget(m, "Minum") != nil {
		t.Fatalf("after deleting the category: %+v", m.Items)
	}
	c.expect(201, "POST", "/api/budgets", M{"categoryId": c.category(domain.TxExpense, "Listrik"), "amount": 50_000})
}
