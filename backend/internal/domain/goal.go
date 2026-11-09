package domain

import (
	"context"
	"time"
)

// Goal is something a user saves towards, e.g. "Laptop baru" Rp 8.000.000.
//
// Progress comes from one of two places:
//   - WalletID set: the balance of that wallet (a dedicated savings wallet), so
//     saving is just moving money there with a transfer;
//   - otherwise: the sum of manual contributions (a virtual piggy bank).
type Goal struct {
	ID           int64      `json:"id"`
	UserID       int64      `json:"-"`
	Name         string     `json:"name"`
	TargetAmount int64      `json:"targetAmount"`
	Deadline     *string    `json:"deadline"` // YYYY-MM-DD
	WalletID     *int64     `json:"walletId"`
	WalletName   string     `json:"walletName,omitempty"`
	Color        string     `json:"color"`
	CreatedAt    time.Time  `json:"createdAt"`
	ArchivedAt   *time.Time `json:"archivedAt"`
	// Contributed is the sum of manual contributions (unused when WalletID is set).
	Contributed int64 `json:"-"`
	// WalletBalance is the linked wallet's balance (when WalletID is set).
	WalletBalance int64 `json:"-"`
}

// GoalProgress is a goal with everything the UI shows about it.
type GoalProgress struct {
	Goal
	Saved     int64 `json:"saved"`
	Remaining int64 `json:"remaining"`
	Percent   int   `json:"percent"`
	Achieved  bool  `json:"achieved"`
	// With a deadline: days left and what to save per week / month to make it.
	DaysLeft     *int  `json:"daysLeft"`
	PerWeek      int64 `json:"perWeek"`
	PerMonth     int64 `json:"perMonth"`
	Overdue      bool  `json:"overdue"`
	ManualSaving bool  `json:"manualSaving"`
}

// Progress computes a goal's state on a given day.
func (g Goal) Progress(today time.Time) GoalProgress {
	p := GoalProgress{Goal: g, Saved: g.Contributed, ManualSaving: g.WalletID == nil}
	if g.WalletID != nil {
		p.Saved = g.WalletBalance
	}
	p.Remaining = max(g.TargetAmount-p.Saved, 0)
	if g.TargetAmount > 0 {
		p.Percent = int(min(p.Saved*100/g.TargetAmount, 100))
	}
	p.Achieved = p.Saved >= g.TargetAmount
	if g.Deadline != nil {
		if d, err := ParseDate(*g.Deadline); err == nil {
			days := int(d.Sub(today).Hours()/24) + 1 // the deadline day counts
			p.DaysLeft = &days
			if !p.Achieved && p.Remaining > 0 {
				if days <= 0 {
					p.Overdue = true
				} else {
					weeks := int64((days + 6) / 7)
					months := int64((days + 29) / 30)
					p.PerWeek = (p.Remaining + weeks - 1) / weeks
					p.PerMonth = (p.Remaining + months - 1) / months
				}
			}
		}
	}
	return p
}

// GoalContribution is one manual deposit (positive) or withdrawal (negative).
type GoalContribution struct {
	ID         int64     `json:"id"`
	GoalID     int64     `json:"goalId"`
	Amount     int64     `json:"amount"`
	Note       string    `json:"note"`
	OccurredOn string    `json:"occurredOn"`
	CreatedAt  time.Time `json:"createdAt"`
}

type GoalRepository interface {
	Create(ctx context.Context, g *Goal) error
	Update(ctx context.Context, g *Goal) error
	Delete(ctx context.Context, userID, id int64) (bool, error)
	ByID(ctx context.Context, userID, id int64) (*Goal, error)
	// List returns active goals (or all, archived included) with sums filled.
	List(ctx context.Context, userID int64, includeArchived bool) ([]Goal, error)
	CountActive(ctx context.Context, userID int64) (int, error)
	SetArchived(ctx context.Context, userID, id int64, at *time.Time) (bool, error)
	AddContribution(ctx context.Context, c *GoalContribution) error
	Contributions(ctx context.Context, goalID int64) ([]GoalContribution, error)
	DeleteContribution(ctx context.Context, goalID, id int64) (bool, error)
}
