package service

import (
	"context"
	"errors"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
	"receh/internal/platform/keymutex"
)

const insufficientNote = "Saldo tidak cukup saat jatuh tempo. Akan dicoba lagi otomatis."

type RecurringInput struct {
	WalletID   int64           `json:"walletId"`
	CategoryID int64           `json:"categoryId"`
	Type       model.TxType    `json:"type"`
	Name       string          `json:"name"`
	Amount     int64           `json:"amount"`
	Frequency  model.Frequency `json:"frequency"`
	StartOn    string          `json:"startOn"`
}

type RecurringService struct {
	repo       model.RecurringRepository
	wallets    model.WalletRepository
	categories model.CategoryRepository
	clock      domain.Clock
	locks      *keymutex.Map
	workers    int
	notify     notifier

	// runMu keeps two sweeps from running at once, e.g. the hourly tick
	// overlapping the sweep at startup.
	runMu sync.Mutex
}

func NewRecurringService(r model.RecurringRepository, w model.WalletRepository, c model.CategoryRepository, clock domain.Clock, locks *keymutex.Map, workers int, n domain.ChangeNotifier) *RecurringService {
	return &RecurringService{repo: r, wallets: w, categories: c, clock: clock, locks: locks, workers: workers, notify: notifier{n}}
}

func (s *RecurringService) validate(ctx context.Context, orgID int64, in *RecurringInput) error {
	name, err := cleanName("name", in.Name, 80, "Beri nama, misalnya \"Netflix\".")
	if err != nil {
		return err
	}
	in.Name = name
	if !in.Type.Valid() {
		return domain.Invalid("type", "Pilih pemasukan atau pengeluaran.")
	}
	if !in.Frequency.Valid() {
		return domain.Invalid("frequency", "Pilih seberapa sering.")
	}
	if err := validAmount("amount", in.Amount); err != nil {
		return err
	}
	w, err := s.wallets.ByID(ctx, orgID, in.WalletID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Invalid("walletId", "Pilih dompet.")
		}
		return err
	}
	if w.Archived {
		return domain.Invalid("walletId", "Dompet ini sudah diarsipkan.")
	}
	c, err := s.categories.ByID(ctx, orgID, in.CategoryID)
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return domain.Invalid("categoryId", "Pilih kategori.")
		}
		return err
	}
	if c.Type != in.Type {
		return domain.Invalid("categoryId", "Kategori ini bukan untuk jenis transaksi tersebut.")
	}
	return nil
}

// Create saves the rule. If it starts today, the first occurrence posts right
// away, so the balance is already correct when the person returns to the page.
func (s *RecurringService) Create(ctx context.Context, orgID int64, in RecurringInput) (*model.Recurring, error) {
	defer s.notify.changed(orgID)
	if err := s.validate(ctx, orgID, &in); err != nil {
		return nil, err
	}
	start, err := domain.ParseDate(in.StartOn)
	if err != nil {
		return nil, domain.Invalid("startOn", "Tanggal mulai tidak valid.")
	}
	if in.StartOn < s.clock.Today() {
		return nil, domain.Invalid("startOn", "Tanggal mulai tidak boleh sudah lewat.")
	}
	r := &model.Recurring{
		OrgID: orgID, WalletID: in.WalletID, CategoryID: in.CategoryID, Type: in.Type, Name: in.Name,
		Amount: in.Amount, Frequency: in.Frequency, AnchorDay: start.Day(), NextDue: in.StartOn, Active: true,
	}
	if err := s.repo.Create(ctx, r); err != nil {
		return nil, err
	}
	s.process(ctx, *r, s.clock.Today())
	return s.repo.ByID(ctx, orgID, r.ID)
}

// Update edits the rule. The schedule resets only if the date actually changed.
func (s *RecurringService) Update(ctx context.Context, orgID, id int64, in RecurringInput) (*model.Recurring, error) {
	defer s.notify.changed(orgID)
	r, err := s.repo.ByID(ctx, orgID, id)
	if err != nil {
		return nil, notFoundOr(err, "Jadwal rutin tidak ditemukan.")
	}
	if err := s.validate(ctx, orgID, &in); err != nil {
		return nil, err
	}
	if in.StartOn != r.NextDue {
		start, err := domain.ParseDate(in.StartOn)
		if err != nil {
			return nil, domain.Invalid("startOn", "Tanggal tidak valid.")
		}
		if in.StartOn < s.clock.Today() {
			return nil, domain.Invalid("startOn", "Tanggal tidak boleh sudah lewat.")
		}
		r.NextDue, r.AnchorDay = in.StartOn, start.Day()
	}
	r.WalletID, r.CategoryID, r.Type, r.Name = in.WalletID, in.CategoryID, in.Type, in.Name
	r.Amount, r.Frequency, r.LastError = in.Amount, in.Frequency, ""
	if err := s.repo.Update(ctx, r); err != nil {
		return nil, err
	}
	if r.Active {
		s.process(ctx, *r, s.clock.Today())
	}
	return s.repo.ByID(ctx, orgID, id)
}

// SetActive pauses or resumes. Periods missed while paused are skipped, not
// charged in a lump: pausing Netflix should not bill the months it was paused.
func (s *RecurringService) SetActive(ctx context.Context, orgID, id int64, active bool) (*model.Recurring, error) {
	defer s.notify.changed(orgID)
	r, err := s.repo.ByID(ctx, orgID, id)
	if err != nil {
		return nil, notFoundOr(err, "Jadwal rutin tidak ditemukan.")
	}
	r.Active, r.LastError = active, ""
	if active {
		today := s.clock.Today()
		for r.NextDue < today {
			next, err := model.NextOccurrence(r.NextDue, r.Frequency, r.AnchorDay)
			if err != nil {
				return nil, err
			}
			r.NextDue = next
		}
	}
	if err := s.repo.Update(ctx, r); err != nil {
		return nil, err
	}
	if active {
		s.process(ctx, *r, s.clock.Today())
	}
	return s.repo.ByID(ctx, orgID, id)
}

func (s *RecurringService) Delete(ctx context.Context, orgID, id int64) error {
	defer s.notify.changed(orgID)
	ok, err := s.repo.Delete(ctx, orgID, id)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NotFound("Jadwal rutin tidak ditemukan.")
	}
	return nil
}

func (s *RecurringService) List(ctx context.Context, orgID int64) ([]model.Recurring, error) {
	return s.repo.List(ctx, orgID)
}

// RunDue posts every occurrence due by today across all users, using a small
// pool of goroutines. It returns how many transactions were posted.
func (s *RecurringService) RunDue(ctx context.Context) (int64, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()

	today := s.clock.Today()
	due, err := s.repo.Due(ctx, today)
	if err != nil {
		return 0, err
	}
	jobs := make(chan model.Recurring)
	var posted atomic.Int64
	var wg sync.WaitGroup
	for range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := range jobs {
				posted.Add(s.process(ctx, r, today))
			}
		}()
	}
	for _, r := range due {
		jobs <- r
	}
	close(jobs)
	wg.Wait()
	return posted.Load(), nil
}

// process posts a rule's occurrences up to today, catching up on any missed
// while the server was off. The wallet mutex is held per occurrence so other
// payments are not starved during a long catch-up.
func (s *RecurringService) process(ctx context.Context, r model.Recurring, today string) int64 {
	var posted int64
	defer func() {
		if posted > 0 {
			s.notify.changed(r.OrgID)
		}
	}()
	for r.NextDue <= today {
		next, err := model.NextOccurrence(r.NextDue, r.Frequency, r.AnchorDay)
		if err != nil {
			log.Printf("recurring %d: next date: %v", r.ID, err)
			return posted
		}
		unlock := s.locks.Lock(r.WalletID)
		err = s.repo.PostOccurrence(ctx, &r, r.NextDue, next)
		unlock()
		if errors.Is(err, model.ErrInsufficient) {
			// Stay due: the sweep retries, and the page shows why it waits.
			if err := s.repo.SetLastError(ctx, r.ID, insufficientNote); err != nil {
				log.Printf("recurring %d: set error: %v", r.ID, err)
			}
			return posted
		}
		if err != nil {
			log.Printf("recurring %d: post: %v", r.ID, err)
			return posted
		}
		posted++
		r.NextDue = next
	}
	return posted
}

// RunScheduler sweeps immediately and then at each interval until ctx ends. The
// immediate sweep catches up on anything due while the server was down.
func (s *RecurringService) RunScheduler(ctx context.Context, every time.Duration) {
	sweep := func() {
		if n, err := s.RunDue(ctx); err != nil {
			if ctx.Err() == nil {
				log.Printf("recurring: sweep: %v", err)
			}
		} else if n > 0 {
			log.Printf("recurring: posted %d transaction(s)", n)
		}
	}
	sweep()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}
