package usecase

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"receh/internal/domain"
	"receh/internal/platform/keymutex"
)

type GoalInput struct {
	Name         string `json:"name"`
	TargetAmount int64  `json:"targetAmount"`
	Deadline     string `json:"deadline"` // YYYY-MM-DD, optional
	WalletID     int64  `json:"walletId"` // 0 = manual contributions
	Color        string `json:"color"`
}

type ContributionInput struct {
	Amount     int64  `json:"amount"` // positive deposit, negative withdrawal
	Note       string `json:"note"`
	OccurredOn string `json:"occurredOn"`
}

// GoalService manages savings goals. Withdrawals from a manual goal hold a
// per-goal mutex, so two withdrawals at once cannot take it below zero.
type GoalService struct {
	goals   domain.GoalRepository
	wallets domain.WalletRepository
	clock   domain.Clock
	locks   *keymutex.Map // userLockKey for the plan limit
	perGoal *keymutex.Map // goal ids, separate from wallet ids
	plans   PlanLimits
	notify  notifier
}

func NewGoalService(g domain.GoalRepository, w domain.WalletRepository, clock domain.Clock, locks *keymutex.Map,
	plans PlanLimits, n domain.ChangeNotifier) *GoalService {
	return &GoalService{goals: g, wallets: w, clock: clock, locks: locks, perGoal: keymutex.New(), plans: plans, notify: notifier{n}}
}

func (s *GoalService) Count(ctx context.Context, userID int64, u *domain.Usage) error {
	n, err := s.goals.CountActive(ctx, userID)
	u.Goals = n
	return err
}

func (s *GoalService) today() time.Time {
	d, _ := domain.ParseDate(s.clock.Today())
	return d
}

func (s *GoalService) validate(ctx context.Context, userID int64, in *GoalInput) (*domain.Goal, error) {
	name, err := cleanName("name", in.Name, 60, "Beri nama target, misalnya \"Laptop baru\".")
	if err != nil {
		return nil, err
	}
	if err := validAmount("targetAmount", in.TargetAmount); err != nil {
		return nil, err
	}
	if in.Color == "" {
		in.Color = "c3"
	}
	if err := validColor(in.Color); err != nil {
		return nil, err
	}
	g := &domain.Goal{UserID: userID, Name: name, TargetAmount: in.TargetAmount, Color: in.Color}
	if in.Deadline != "" {
		d, err := domain.ParseDate(in.Deadline)
		if err != nil {
			return nil, domain.Invalid("deadline", "Tanggal tidak valid.")
		}
		if d.Before(s.today()) {
			return nil, domain.Invalid("deadline", "Tenggat tidak boleh sudah lewat.")
		}
		dl := d.Format(domain.DateLayout)
		g.Deadline = &dl
	}
	if in.WalletID != 0 {
		w, err := s.wallets.ByID(ctx, userID, in.WalletID)
		if errors.Is(err, domain.ErrNoRows) {
			return nil, domain.Invalid("walletId", "Pilih dompet.")
		}
		if err != nil {
			return nil, err
		}
		if w.Archived {
			return nil, domain.Invalid("walletId", "Dompet ini sudah diarsipkan.")
		}
		g.WalletID = &w.ID
	}
	return g, nil
}

func (s *GoalService) List(ctx context.Context, userID int64, includeArchived bool) ([]domain.GoalProgress, error) {
	goals, err := s.goals.List(ctx, userID, includeArchived)
	if err != nil {
		return nil, err
	}
	today := s.today()
	out := make([]domain.GoalProgress, 0, len(goals))
	for _, g := range goals {
		out = append(out, g.Progress(today))
	}
	return out, nil
}

func (s *GoalService) get(ctx context.Context, userID, id int64) (*domain.Goal, error) {
	g, err := s.goals.ByID(ctx, userID, id)
	if err != nil {
		return nil, notFoundOr(err, "Target tidak ditemukan.")
	}
	return g, nil
}

func (s *GoalService) progress(ctx context.Context, userID, id int64) (*domain.GoalProgress, error) {
	g, err := s.get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	p := g.Progress(s.today())
	return &p, nil
}

func (s *GoalService) Create(ctx context.Context, userID int64, in GoalInput) (*domain.GoalProgress, error) {
	defer s.notify.changed(userID)
	g, err := s.validate(ctx, userID, &in)
	if err != nil {
		return nil, err
	}
	unlock := s.locks.Lock(userLockKey(userID))
	defer unlock()
	if err := s.checkLimit(ctx, userID); err != nil {
		return nil, err
	}
	if err := s.goals.Create(ctx, g); err != nil {
		return nil, err
	}
	return s.progress(ctx, userID, g.ID)
}

func (s *GoalService) checkLimit(ctx context.Context, userID int64) error {
	limits, err := s.plans.Limits(ctx, userID)
	if err != nil || limits.MaxGoals == domain.Unlimited {
		return err
	}
	n, err := s.goals.CountActive(ctx, userID)
	if err != nil {
		return err
	}
	if !domain.Allows(limits.MaxGoals, n) {
		return domain.PlanLimit("Paket Gratis bisa memakai " + itoa(limits.MaxGoals) + " target tabungan aktif. Upgrade ke Premium untuk target tanpa batas, atau arsipkan target yang sudah selesai.")
	}
	return nil
}

func (s *GoalService) Update(ctx context.Context, userID, id int64, in GoalInput) (*domain.GoalProgress, error) {
	defer s.notify.changed(userID)
	cur, err := s.get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	g, err := s.validate(ctx, userID, &in)
	if err != nil {
		return nil, err
	}
	// Switching from manual saving to a wallet would silently ignore the
	// contributions already made; ask the user to settle them first.
	if cur.WalletID == nil && g.WalletID != nil && cur.Contributed != 0 {
		return nil, domain.Invalid("walletId", "Target ini sudah punya setoran manual. Tarik dulu setorannya sebelum menautkan dompet.")
	}
	g.ID = id
	if err := s.goals.Update(ctx, g); err != nil {
		return nil, err
	}
	return s.progress(ctx, userID, id)
}

func (s *GoalService) Delete(ctx context.Context, userID, id int64) error {
	defer s.notify.changed(userID)
	ok, err := s.goals.Delete(ctx, userID, id)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NotFound("Target tidak ditemukan.")
	}
	return nil
}

// SetArchived moves a finished goal out of the way, or brings it back (which
// counts against the plan limit again).
func (s *GoalService) SetArchived(ctx context.Context, userID, id int64, archived bool) (*domain.GoalProgress, error) {
	defer s.notify.changed(userID)
	var at *time.Time
	if archived {
		now := s.clock.Now()
		at = &now
	} else {
		unlock := s.locks.Lock(userLockKey(userID))
		defer unlock()
		if err := s.checkLimit(ctx, userID); err != nil {
			return nil, err
		}
	}
	ok, err := s.goals.SetArchived(ctx, userID, id, at)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.NotFound("Target tidak ditemukan.")
	}
	return s.progress(ctx, userID, id)
}

func (s *GoalService) Contribute(ctx context.Context, userID, id int64, in ContributionInput) (*domain.GoalProgress, error) {
	defer s.notify.changed(userID)
	if in.Amount == 0 {
		return nil, domain.Invalid("amount", "Masukkan nominalnya.")
	}
	if err := validAmount("amount", abs64(in.Amount)); err != nil {
		return nil, err
	}
	in.Note = strings.Join(strings.Fields(in.Note), " ")
	if utf8.RuneCountInString(in.Note) > 80 {
		return nil, domain.Invalid("note", "Catatan maksimal 80 karakter.")
	}
	if in.OccurredOn == "" {
		in.OccurredOn = s.clock.Today()
	}
	on, err := domain.ParseDate(in.OccurredOn)
	if err != nil {
		return nil, domain.Invalid("occurredOn", "Tanggal tidak valid.")
	}
	if on.After(s.today().AddDate(0, 0, 1)) {
		return nil, domain.Invalid("occurredOn", "Tanggal tidak boleh di masa depan.")
	}

	unlock := s.perGoal.Lock(id)
	defer unlock()
	g, err := s.get(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if g.WalletID != nil {
		return nil, domain.Invalid("", "Target ini mengikuti saldo dompet "+g.WalletName+". Pindahkan uang ke dompet itu dengan Transfer.")
	}
	if g.ArchivedAt != nil {
		return nil, domain.Invalid("", "Target ini sudah diarsipkan.")
	}
	if g.Contributed+in.Amount < 0 {
		return nil, domain.Invalid("amount", "Tabungan target ini hanya "+formatRp(g.Contributed)+".")
	}
	c := &domain.GoalContribution{GoalID: id, Amount: in.Amount, Note: in.Note, OccurredOn: on.Format(domain.DateLayout)}
	if err := s.goals.AddContribution(ctx, c); err != nil {
		return nil, err
	}
	return s.progress(ctx, userID, id)
}

func (s *GoalService) Contributions(ctx context.Context, userID, id int64) ([]domain.GoalContribution, error) {
	if _, err := s.get(ctx, userID, id); err != nil {
		return nil, err
	}
	return s.goals.Contributions(ctx, id)
}

func (s *GoalService) DeleteContribution(ctx context.Context, userID, goalID, id int64) (*domain.GoalProgress, error) {
	defer s.notify.changed(userID)
	unlock := s.perGoal.Lock(goalID)
	defer unlock()
	g, err := s.get(ctx, userID, goalID)
	if err != nil {
		return nil, err
	}
	list, err := s.goals.Contributions(ctx, goalID)
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		if c.ID == id && g.Contributed-c.Amount < 0 {
			return nil, domain.Invalid("", "Menghapus setoran ini membuat tabungan minus. Hapus penarikannya dulu.")
		}
	}
	ok, err := s.goals.DeleteContribution(ctx, goalID, id)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, domain.NotFound("Setoran tidak ditemukan.")
	}
	return s.progress(ctx, userID, goalID)
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}
