package service

import (
	"context"

	"receh/internal/domain"
	"receh/internal/kit"
)

// Plan-limit keys of the finance module, registered in module.go.
const (
	LimitWallets   = "finance.wallets"
	LimitBudgets   = "finance.budgets"
	LimitGoals     = "finance.goals"
	LimitCSVImport = "finance.csv_import"
)

// MaxAmount caps one amount (see kit.MaxAmount).
const MaxAmount = kit.MaxAmount

const kitMaxAmount = kit.MaxAmount

// PlanLimits is how the module asks the core what a user's plan allows.
type PlanLimits = kit.PlanLimits

// Short local names for the shared helpers in package kit.
var (
	parallel    = kit.Parallel
	notFoundOr  = kit.NotFoundOr
	cleanName   = kit.CleanName
	itoa        = kit.Itoa
	itoa64      = kit.Itoa64
	formatRp    = kit.FormatRp
	validAmount = kit.ValidAmount
	validColor  = kit.ValidColor
	userLockKey = kit.UserLockKey
)

// notifier wraps kit.Notifier with the lower-case method the services use.
type notifier struct{ n domain.ChangeNotifier }

func (x notifier) changed(userID int64) { kit.Notifier{N: x.n}.Changed(userID) }

// checkLimit refuses one more item when count already reaches the limit. The
// count only runs when the plan has a limit at all.
func checkLimit(ctx context.Context, p PlanLimits, userID int64, key string, count func() (int, error), msg string) error {
	limit, err := p.Limit(ctx, userID, key)
	if err != nil || limit == domain.Unlimited {
		return err
	}
	n, err := count()
	if err != nil {
		return err
	}
	return kit.CheckLimit(ctx, constLimit(limit), userID, key, n, msg)
}

type constLimit int

func (c constLimit) Limit(context.Context, int64, string) (int, error) { return int(c), nil }
