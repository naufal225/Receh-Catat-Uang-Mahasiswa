package usecase

import (
	"context"
	"errors"
	"strconv"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/keymutex"
)

type BudgetInput struct {
	CategoryID int64 `json:"categoryId"`
	Amount     int64 `json:"amount"`
}

// BudgetService manages monthly spending caps per expense category. Spending
// is never stored twice: it is summed from transactions for the month asked,
// so a budget is always consistent with history, edits and deletions included.
type BudgetService struct {
	budgets    domain.BudgetRepository
	categories domain.CategoryRepository
	txs        domain.TransactionRepository
	clock      domain.Clock
	locks      *keymutex.Map
	plans      PlanLimits
	notify     notifier
}

func NewBudgetService(b domain.BudgetRepository, c domain.CategoryRepository, t domain.TransactionRepository,
	clock domain.Clock, locks *keymutex.Map, plans PlanLimits, n domain.ChangeNotifier) *BudgetService {
	return &BudgetService{budgets: b, categories: c, txs: t, clock: clock, locks: locks, plans: plans, notify: notifier{n}}
}

// Count is registered with MembershipService to show usage against the limit.
func (s *BudgetService) Count(ctx context.Context, userID int64, u *domain.Usage) error {
	n, err := s.budgets.Count(ctx, userID)
	u.Budgets = n
	return err
}

func (s *BudgetService) Create(ctx context.Context, userID int64, in BudgetInput) (*domain.Budget, error) {
	defer s.notify.changed(userID)
	if err := validAmount("amount", in.Amount); err != nil {
		return nil, err
	}
	c, err := s.categories.ByID(ctx, userID, in.CategoryID)
	if errors.Is(err, domain.ErrNoRows) {
		return nil, domain.Invalid("categoryId", "Pilih kategori.")
	}
	if err != nil {
		return nil, err
	}
	if c.Type != domain.TxExpense {
		return nil, domain.Invalid("categoryId", "Anggaran hanya untuk kategori pengeluaran.")
	}
	unlock := s.locks.Lock(userLockKey(userID))
	defer unlock()
	limits, err := s.plans.Limits(ctx, userID)
	if err != nil {
		return nil, err
	}
	if limits.MaxBudgets != domain.Unlimited {
		n, err := s.budgets.Count(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !domain.Allows(limits.MaxBudgets, n) {
			return nil, domain.PlanLimit("Paket Gratis bisa memakai " + itoa(limits.MaxBudgets) + " anggaran. Upgrade ke Premium untuk anggaran tanpa batas.")
		}
	}
	b := &domain.Budget{UserID: userID, CategoryID: c.ID, CategoryName: c.Name, CategoryColor: c.Color, Amount: in.Amount}
	if err := s.budgets.Create(ctx, b); err != nil {
		return nil, err
	}
	return b, nil
}

func (s *BudgetService) Update(ctx context.Context, userID, id, amount int64) (*domain.Budget, error) {
	defer s.notify.changed(userID)
	if err := validAmount("amount", amount); err != nil {
		return nil, err
	}
	ok, err := s.budgets.UpdateAmount(ctx, userID, id, amount)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.NotFound("Anggaran tidak ditemukan.")
	}
	return s.budgets.ByID(ctx, userID, id)
}

func (s *BudgetService) Delete(ctx context.Context, userID, id int64) error {
	defer s.notify.changed(userID)
	ok, err := s.budgets.Delete(ctx, userID, id)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NotFound("Anggaran tidak ditemukan.")
	}
	return nil
}

// Month reports every budget against spending in one month ("YYYY-MM"; empty
// means the current month).
func (s *BudgetService) Month(ctx context.Context, userID int64, month string) (*domain.BudgetMonth, error) {
	today, err := domain.ParseDate(s.clock.Today())
	if err != nil {
		return nil, err
	}
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, time.UTC)
	if month != "" {
		if first, err = time.Parse("2006-01", month); err != nil {
			return nil, domain.Invalid("month", "Bulan tidak valid.")
		}
	}
	last := first.AddDate(0, 1, -1)
	from, to := first.Format(domain.DateLayout), last.Format(domain.DateLayout)

	var budgets []domain.Budget
	var spent []domain.CategorySlice
	err = parallel(
		func() (e error) { budgets, e = s.budgets.List(ctx, userID); return },
		func() (e error) { spent, e = s.txs.SumByCategory(ctx, userID, 0, domain.TxExpense, from, to); return },
	)
	if err != nil {
		return nil, err
	}
	byCat := make(map[int64]int64, len(spent))
	var allSpent int64
	for _, sl := range spent {
		byCat[sl.CategoryID] = sl.Total
		allSpent += sl.Total
	}

	out := &domain.BudgetMonth{
		Month: first.Format("2006-01"), Title: monthsLong[first.Month()-1] + " " + strconv.Itoa(first.Year()),
		Items: make([]domain.BudgetStatus, 0, len(budgets)),
	}
	if first.Year() == today.Year() && first.Month() == today.Month() {
		out.IsCurrent = true
		out.DaysLeft = last.Day() - today.Day() + 1
	}
	for _, b := range budgets {
		used := byCat[b.CategoryID]
		st := domain.BudgetStatus{Budget: b, Spent: used, Remaining: b.Amount - used, Level: domain.LevelFor(used, b.Amount)}
		st.Percent = int(used * 100 / b.Amount)
		out.Items = append(out.Items, st)
		out.TotalBudget += b.Amount
		out.TotalSpent += used
	}
	out.Unbudgeted = allSpent - out.TotalSpent
	return out, nil
}
