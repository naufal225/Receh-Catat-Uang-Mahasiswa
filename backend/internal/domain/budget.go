package domain

import (
	"context"
	"time"
)

// Budget caps spending in one expense category per calendar month.
type Budget struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"-"`
	CategoryID    int64     `json:"categoryId"`
	CategoryName  string    `json:"categoryName"`
	CategoryColor string    `json:"categoryColor"`
	Amount        int64     `json:"amount"`
	CreatedAt     time.Time `json:"createdAt"`
}

// BudgetLevel says how close spending is to the budget.
type BudgetLevel string

const (
	BudgetOK      BudgetLevel = "ok"      // below 80%
	BudgetWarning BudgetLevel = "warning" // 80% up to 100%
	BudgetOver    BudgetLevel = "over"    // above 100%
)

// BudgetWarnAt is the share of a budget after which it is shown as a warning.
const BudgetWarnAt = 0.8

// LevelFor classifies spending against an amount.
func LevelFor(spent, amount int64) BudgetLevel {
	switch {
	case amount <= 0 || spent > amount:
		return BudgetOver
	case float64(spent) >= BudgetWarnAt*float64(amount):
		return BudgetWarning
	}
	return BudgetOK
}

// BudgetStatus is a budget with what has been spent in one month.
type BudgetStatus struct {
	Budget
	Spent     int64       `json:"spent"`
	Remaining int64       `json:"remaining"`
	Percent   int         `json:"percent"`
	Level     BudgetLevel `json:"level"`
}

// BudgetMonth is every budget of a user for one month, with totals.
type BudgetMonth struct {
	Month        string         `json:"month"`
	Title        string         `json:"title"`
	Items        []BudgetStatus `json:"items"`
	TotalBudget  int64          `json:"totalBudget"`
	TotalSpent   int64          `json:"totalSpent"`
	DaysLeft     int            `json:"daysLeft"`
	IsCurrent    bool           `json:"isCurrent"`
	// Unbudgeted is spending this month in categories without a budget.
	Unbudgeted int64 `json:"unbudgeted"`
}

type BudgetRepository interface {
	Create(ctx context.Context, b *Budget) error
	UpdateAmount(ctx context.Context, userID, id, amount int64) (bool, error)
	Delete(ctx context.Context, userID, id int64) (bool, error)
	ByID(ctx context.Context, userID, id int64) (*Budget, error)
	List(ctx context.Context, userID int64) ([]Budget, error)
	Count(ctx context.Context, userID int64) (int, error)
}
