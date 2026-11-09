package usecase

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/cache"
)

var monthsShort = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

var monthsLong = []string{"Januari", "Februari", "Maret", "April", "Mei", "Juni", "Juli", "Agustus", "September", "Oktober", "November", "Desember"}

type DashboardService struct {
	txs       domain.TransactionRepository
	wallets   domain.WalletRepository
	recurring domain.RecurringRepository
	clock     domain.Clock

	// Built dashboards, keyed by everything that shapes one, including the
	// user's data version: any change bumps it, so a stale entry is simply
	// never asked for again and ages out.
	cache    *cache.TTL[dashKey, *domain.Dashboard]
	versions *cache.Versions
	budgets  BudgetMonths
	goals    GoalLister
}

// BudgetMonths is what the dashboard needs from BudgetService.
type BudgetMonths interface {
	Month(ctx context.Context, userID int64, month string) (*domain.BudgetMonth, error)
}

// WithBudgets adds this month's budgets to the dashboard.
func (s *DashboardService) WithBudgets(b BudgetMonths) { s.budgets = b }

// GoalLister is what the dashboard needs from GoalService.
type GoalLister interface {
	List(ctx context.Context, userID int64, includeArchived bool) ([]domain.GoalProgress, error)
}

// WithGoals adds active savings goals to the dashboard.
func (s *DashboardService) WithGoals(g GoalLister) { s.goals = g }

type dashKey struct {
	userID, walletID int64
	version          uint64
	unit, today      string
	offset           int
}

// NewDashboardService takes the version counter the write services bump; a
// zero ttl turns the cache off.
func NewDashboardService(t domain.TransactionRepository, w domain.WalletRepository, r domain.RecurringRepository, clock domain.Clock, versions *cache.Versions, ttl time.Duration) *DashboardService {
	return &DashboardService{txs: t, wallets: w, recurring: r, clock: clock,
		cache: cache.New[dashKey, *domain.Dashboard](ttl, 5000), versions: versions}
}

// Cache exposes the cache so the app can run its janitor goroutine.
func (s *DashboardService) Cache() *cache.TTL[dashKey, *domain.Dashboard] { return s.cache }

// Build returns the dashboard, from cache when nothing changed since it was
// last built. Cached values are shared, so callers must not modify them.
func (s *DashboardService) Build(ctx context.Context, userID int64, unit string, offset int, walletID int64) (*domain.Dashboard, error) {
	if offset < 0 {
		offset = 0
	}
	key := dashKey{userID: userID, walletID: walletID, unit: unit, offset: offset, today: s.clock.Today()}
	if s.versions != nil {
		key.version = s.versions.Get(userID)
	}
	if d, ok := s.cache.Get(key); ok {
		return d, nil
	}
	d, err := s.build(ctx, userID, unit, offset, walletID)
	if err != nil {
		return nil, err
	}
	// Store under the version read before building: if a write landed
	// meanwhile, the version has moved on and this entry is never served.
	s.cache.Set(key, d)
	return d, nil
}

func dayLabel(d time.Time) string {
	return strconv.Itoa(d.Day()) + " " + monthsShort[d.Month()-1]
}

// buildBuckets returns the empty buckets of one window of a time unit. Offset 0
// is the window containing today; offset 1 is the one before it, and so on.
func buildBuckets(unit string, today time.Time, offset int) ([]domain.Bucket, string, error) {
	var out []domain.Bucket
	add := func(from, to time.Time, label string) {
		out = append(out, domain.Bucket{
			Key: from.Format(domain.DateLayout), Label: label,
			From: from.Format(domain.DateLayout), To: to.Format(domain.DateLayout),
		})
	}
	switch unit {
	case "day":
		const n = 14
		end := today.AddDate(0, 0, -n*offset)
		for i := n - 1; i >= 0; i-- {
			d := end.AddDate(0, 0, -i)
			add(d, d, dayLabel(d))
		}
	case "week":
		const n = 12
		monday := today.AddDate(0, 0, -(int(today.Weekday())+6)%7)
		end := monday.AddDate(0, 0, -7*n*offset)
		for i := n - 1; i >= 0; i-- {
			from := end.AddDate(0, 0, -7*i)
			add(from, from.AddDate(0, 0, 6), dayLabel(from))
		}
	case "month":
		const n = 12
		first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
		end := first.AddDate(0, -n*offset, 0)
		for i := n - 1; i >= 0; i-- {
			from := end.AddDate(0, -i, 0)
			add(from, from.AddDate(0, 1, -1), monthsShort[from.Month()-1])
		}
	case "year":
		const n = 5
		endYear := today.Year() - n*offset
		for i := n - 1; i >= 0; i-- {
			y := endYear - i
			add(time.Date(y, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(y, 12, 31, 0, 0, 0, 0, time.UTC), strconv.Itoa(y))
		}
	default:
		return nil, "", domain.Invalid("unit", "Satuan waktu tidak dikenal.")
	}
	return out, windowTitle(unit, out), nil
}

func windowTitle(unit string, b []domain.Bucket) string {
	first, _ := domain.ParseDate(b[0].From)
	last, _ := domain.ParseDate(b[len(b)-1].To)
	switch unit {
	case "year":
		return fmt.Sprintf("%d – %d", first.Year(), last.Year())
	case "month":
		return fmt.Sprintf("%s %d – %s %d", monthsShort[first.Month()-1], first.Year(), monthsShort[last.Month()-1], last.Year())
	}
	return fmt.Sprintf("%s – %s %d", dayLabel(first), dayLabel(last), last.Year())
}

// build assembles the dashboard, running its independent queries in parallel
// goroutines. Balances and latest transactions follow the wallet filter.
func (s *DashboardService) build(ctx context.Context, userID int64, unit string, offset int, walletID int64) (*domain.Dashboard, error) {
	if offset < 0 {
		offset = 0
	}
	today, err := domain.ParseDate(s.clock.Today())
	if err != nil {
		return nil, err
	}
	buckets, title, err := buildBuckets(unit, today, offset)
	if err != nil {
		return nil, err
	}
	from, to := buckets[0].From, buckets[len(buckets)-1].To
	d := &domain.Dashboard{Unit: unit, From: from, To: to, Title: title, CanGoForward: offset > 0}

	var days []domain.DaySum
	err = parallel(
		func() (e error) { days, e = s.txs.SumByDay(ctx, userID, walletID, from, to); return },
		func() (e error) {
			d.ExpenseSlices, e = s.txs.SumByCategory(ctx, userID, walletID, domain.TxExpense, from, to)
			return
		},
		func() (e error) {
			d.IncomeSlices, e = s.txs.SumByCategory(ctx, userID, walletID, domain.TxIncome, from, to)
			return
		},
		func() (e error) { d.Wallets, e = s.wallets.List(ctx, userID, false); return },
		func() (e error) { d.Latest, e = s.txs.Latest(ctx, userID, walletID, 8); return },
		func() (e error) {
			d.Goals = []domain.GoalProgress{}
			if s.goals != nil {
				d.Goals, e = s.goals.List(ctx, userID, false)
			}
			return
		},
		func() error {
			d.Budgets = []domain.BudgetStatus{}
			if s.budgets == nil {
				return nil
			}
			m, e := s.budgets.Month(ctx, userID, "")
			if e == nil {
				d.Budgets = m.Items
			}
			return e
		},
		func() error {
			all, e := s.recurring.List(ctx, userID)
			if e != nil {
				return e
			}
			d.UpcomingRoutines = []domain.Recurring{}
			for _, r := range all {
				if r.Active && len(d.UpcomingRoutines) < 3 {
					d.UpcomingRoutines = append(d.UpcomingRoutines, r)
				}
			}
			return nil
		},
	)
	if err != nil {
		return nil, err
	}

	for _, day := range days {
		for i := range buckets {
			if day.Day >= buckets[i].From && day.Day <= buckets[i].To {
				buckets[i].Income += day.Income
				buckets[i].Expense += day.Expense
				break
			}
		}
		d.Totals.Income += day.Income
		d.Totals.Expense += day.Expense
	}
	d.Buckets = buckets
	for _, w := range d.Wallets {
		if walletID == 0 || w.ID == walletID {
			d.TotalBalance += w.Balance
		}
	}
	return d, nil
}

// SliceSet is the category breakdown of one arbitrary range, used when a bar is
// clicked to focus the pie chart on that period.
type SliceSet struct {
	Totals        domain.Totals          `json:"totals"`
	ExpenseSlices []domain.CategorySlice `json:"expenseSlices"`
	IncomeSlices  []domain.CategorySlice `json:"incomeSlices"`
}

func (s *DashboardService) Slices(ctx context.Context, userID, walletID int64, from, to string) (*SliceSet, error) {
	if _, err := domain.ParseDate(from); err != nil {
		return nil, domain.Invalid("from", "Tanggal tidak valid.")
	}
	if _, err := domain.ParseDate(to); err != nil {
		return nil, domain.Invalid("to", "Tanggal tidak valid.")
	}
	out := &SliceSet{}
	err := parallel(
		func() (e error) {
			out.ExpenseSlices, e = s.txs.SumByCategory(ctx, userID, walletID, domain.TxExpense, from, to)
			return
		},
		func() (e error) {
			out.IncomeSlices, e = s.txs.SumByCategory(ctx, userID, walletID, domain.TxIncome, from, to)
			return
		},
	)
	if err != nil {
		return nil, err
	}
	for _, c := range out.ExpenseSlices {
		out.Totals.Expense += c.Total
	}
	for _, c := range out.IncomeSlices {
		out.Totals.Income += c.Total
	}
	return out, nil
}
