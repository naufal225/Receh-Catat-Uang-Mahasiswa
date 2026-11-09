package usecase

import (
	"context"
	"log"
	"strconv"
	"sync"
	"time"

	"receh/internal/domain"
)

// NotificationService writes in-app reminders. Two things drive it:
//
//   - Changed(userID), called after every write (it is a ChangeNotifier), puts
//     the user on a channel. A worker goroutine re-checks that user's
//     event-driven reminders (budgets, low balance, goals, failed bills), so a
//     budget warning appears right after the expense that caused it.
//   - RunScheduler sweeps every active user on a ticker with a small goroutine
//     pool, adding the time-driven ones (bill due tomorrow, evening reminder,
//     Monday summary).
//
// Every notification carries a ref key that is unique per user in the
// database, so evaluating twice, or after a restart, never notifies twice.
type NotificationService struct {
	repo      domain.NotificationRepository
	users     domain.UserRepository
	wallets   domain.WalletRepository
	recurring domain.RecurringRepository
	txs       domain.TransactionRepository
	budgets   BudgetMonths
	goals     GoalLister
	clock     domain.Clock

	queue   chan int64
	mu      sync.Mutex
	pending map[int64]bool // users already queued, so bursts of writes coalesce
	workers int
	runMu   sync.Mutex // one sweep at a time
}

func NewNotificationService(r domain.NotificationRepository, u domain.UserRepository, w domain.WalletRepository,
	rec domain.RecurringRepository, t domain.TransactionRepository, b BudgetMonths, g GoalLister, clock domain.Clock) *NotificationService {
	return &NotificationService{
		repo: r, users: u, wallets: w, recurring: rec, txs: t, budgets: b, goals: g, clock: clock,
		queue: make(chan int64, 1024), pending: map[int64]bool{}, workers: 4,
	}
}

// Changed implements domain.ChangeNotifier. It never blocks a request: when
// the queue is full the check is skipped and the next sweep catches up.
func (s *NotificationService) Changed(userID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[userID] {
		return
	}
	select {
	case s.queue <- userID:
		s.pending[userID] = true
	default:
	}
}

// RunWorker processes queued users until ctx ends. Run it in its own goroutine.
func (s *NotificationService) RunWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case uid := <-s.queue:
			s.mu.Lock()
			delete(s.pending, uid)
			s.mu.Unlock()
			if _, err := s.Evaluate(ctx, uid, false); err != nil && ctx.Err() == nil {
				log.Printf("notifications: user %d: %v", uid, err)
			}
		}
	}
}

// RunScheduler sweeps now and then on every tick until ctx ends.
func (s *NotificationService) RunScheduler(ctx context.Context, every time.Duration) {
	sweep := func() {
		if n, err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			log.Printf("notifications: sweep: %v", err)
		} else if n > 0 {
			log.Printf("notifications: %d new", n)
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

// Sweep evaluates every active user, time-driven reminders included, and
// drops notifications older than 90 days. It returns how many were created.
func (s *NotificationService) Sweep(ctx context.Context) (int, error) {
	s.runMu.Lock()
	defer s.runMu.Unlock()
	ids, err := s.users.ActiveUserIDs(ctx)
	if err != nil {
		return 0, err
	}
	jobs := make(chan int64)
	var created int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for range s.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for uid := range jobs {
				n, err := s.Evaluate(ctx, uid, true)
				if err != nil {
					log.Printf("notifications: user %d: %v", uid, err)
				}
				mu.Lock()
				created += n
				mu.Unlock()
			}
		}()
	}
	for _, id := range ids {
		jobs <- id
	}
	close(jobs)
	wg.Wait()
	if _, err := s.repo.DeleteBefore(ctx, s.clock.Now().AddDate(0, 0, -90)); err != nil {
		return created, err
	}
	return created, nil
}

// Evaluate checks one user. timeDriven adds reminders that depend on the clock
// rather than on a change (bill due tomorrow, evening reminder, weekly summary).
func (s *NotificationService) Evaluate(ctx context.Context, userID int64, timeDriven bool) (int, error) {
	prefs, err := s.repo.Prefs(ctx, userID)
	if err != nil {
		return 0, err
	}
	var all []domain.Notification
	add := func(kind domain.NotificationKind, ref, title, body, link string) {
		if prefs.Enabled(kind) {
			all = append(all, domain.Notification{UserID: userID, Kind: kind, RefKey: ref, Title: title, Body: body, Link: link})
		}
	}
	today := s.clock.Today()
	day, _ := domain.ParseDate(today)

	// Budgets: one notice per budget per month and level.
	if m, err := s.budgets.Month(ctx, userID, ""); err != nil {
		return 0, err
	} else {
		for _, b := range m.Items {
			id := strconv.FormatInt(b.ID, 10)
			switch b.Level {
			case domain.BudgetOver:
				add(domain.NotifyBudgetOver, "budget_over:"+id+":"+m.Month, "Anggaran "+b.CategoryName+" terlewati",
					"Terpakai "+formatRp(b.Spent)+" dari "+formatRp(b.Amount)+" ("+itoa(b.Percent)+"%) bulan ini.", "/app/anggaran")
			case domain.BudgetWarning:
				add(domain.NotifyBudgetWarning, "budget_warning:"+id+":"+m.Month, "Anggaran "+b.CategoryName+" hampir habis",
					"Sisa "+formatRp(b.Remaining)+" ("+itoa(b.Percent)+"% terpakai), "+itoa(m.DaysLeft)+" hari lagi bulan ini.", "/app/anggaran")
			}
		}
	}

	// Wallets: at most one low-balance notice per wallet per day, and only for
	// wallets in use (a brand-new empty wallet is not news).
	wallets, err := s.wallets.List(ctx, userID, false)
	if err != nil {
		return 0, err
	}
	balance := map[int64]domain.Wallet{}
	for _, w := range wallets {
		balance[w.ID] = w
		if prefs.LowBalanceThreshold > 0 && w.Balance < prefs.LowBalanceThreshold {
			used, err := s.wallets.HasTransactions(ctx, w.ID)
			if err != nil {
				return 0, err
			}
			if used {
				add(domain.NotifyLowBalance, "low_balance:"+strconv.FormatInt(w.ID, 10)+":"+today, "Saldo "+w.Name+" menipis",
					"Tinggal "+formatRp(w.Balance)+", di bawah batas "+formatRp(prefs.LowBalanceThreshold)+" yang kamu atur.", "/app/dompet")
			}
		}
	}

	// Goals reached: once per goal.
	goals, err := s.goals.List(ctx, userID, false)
	if err != nil {
		return 0, err
	}
	for _, g := range goals {
		if g.Achieved {
			add(domain.NotifyGoalAchieved, "goal_achieved:"+strconv.FormatInt(g.ID, 10), "Target "+g.Name+" tercapai!",
				"Kamu sudah mengumpulkan "+formatRp(g.Saved)+". Saatnya merayakan, atau arsipkan targetnya.", "/app/target")
		}
	}

	// Recurring bills: failed for lack of money (event), due tomorrow (time).
	rules, err := s.recurring.List(ctx, userID)
	if err != nil {
		return 0, err
	}
	tomorrow := day.AddDate(0, 0, 1).Format(domain.DateLayout)
	for _, r := range rules {
		if !r.Active || r.Type != domain.TxExpense {
			continue
		}
		id := strconv.FormatInt(r.ID, 10)
		if r.LastError != "" {
			add(domain.NotifyBillFailed, "bill_failed:"+id+":"+r.NextDue, r.Name+" belum bisa dicatat",
				"Saldo "+r.WalletName+" tidak cukup untuk "+formatRp(r.Amount)+". Receh mencoba lagi otomatis setelah saldo cukup.", "/app/rutin")
		}
		if timeDriven && r.NextDue == tomorrow {
			body := formatRp(r.Amount) + " dari " + r.WalletName + " akan tercatat otomatis besok."
			if w, ok := balance[r.WalletID]; ok && w.Balance < r.Amount {
				body += " Saldo " + w.Name + " sekarang " + formatRp(w.Balance) + ", belum cukup."
			}
			add(domain.NotifyBillDue, "bill_due:"+id+":"+r.NextDue, "Tagihan besok: "+r.Name, body, "/app/rutin")
		}
	}

	if timeDriven && len(wallets) > 0 {
		now := s.clock.Now()
		// Evening nudge when nothing was recorded today.
		if now.Hour() >= prefs.ReminderHour {
			days, err := s.txs.SumByDay(ctx, userID, 0, today, today)
			if err != nil {
				return 0, err
			}
			if len(days) == 0 {
				add(domain.NotifyDailyReminder, "daily:"+today, "Sudah mencatat hari ini?",
					"Belum ada pemasukan atau pengeluaran hari ini. Catat sekarang selagi masih ingat.", "/app")
			}
		}
		// Monday: last week in numbers, compared with the week before.
		if day.Weekday() == time.Monday {
			if n, ok, err := s.weekly(ctx, userID, day); err != nil {
				return 0, err
			} else if ok {
				add(domain.NotifyWeeklySummary, n.RefKey, n.Title, n.Body, n.Link)
			}
		}
	}

	created := 0
	for i := range all {
		ok, err := s.repo.Insert(ctx, &all[i])
		if err != nil {
			return created, err
		}
		if ok {
			created++
		}
	}
	return created, nil
}

func (s *NotificationService) weekly(ctx context.Context, userID int64, monday time.Time) (domain.Notification, bool, error) {
	from, to := monday.AddDate(0, 0, -7), monday.AddDate(0, 0, -1)
	sum := func(a, b time.Time) (in, out int64, err error) {
		days, err := s.txs.SumByDay(ctx, userID, 0, a.Format(domain.DateLayout), b.Format(domain.DateLayout))
		for _, d := range days {
			in += d.Income
			out += d.Expense
		}
		return in, out, err
	}
	in, out, err := sum(from, to)
	if err != nil || in+out == 0 {
		return domain.Notification{}, false, err
	}
	_, prevOut, err := sum(from.AddDate(0, 0, -7), from.AddDate(0, 0, -1))
	if err != nil {
		return domain.Notification{}, false, err
	}
	body := "Masuk " + formatRp(in) + ", keluar " + formatRp(out) + "."
	cats, err := s.txs.SumByCategory(ctx, userID, 0, domain.TxExpense, from.Format(domain.DateLayout), to.Format(domain.DateLayout))
	if err != nil {
		return domain.Notification{}, false, err
	}
	if len(cats) > 0 {
		body += " Terbesar: " + cats[0].Name + " " + formatRp(cats[0].Total) + "."
	}
	if prevOut > 0 && out != prevOut {
		pct := (out - prevOut) * 100 / prevOut
		if pct > 0 {
			body += " Pengeluaran naik " + itoa64(pct) + "% dari minggu sebelumnya."
		} else if pct < 0 {
			body += " Pengeluaran turun " + itoa64(-pct) + "% dari minggu sebelumnya."
		}
	}
	return domain.Notification{
		RefKey: "weekly:" + from.Format(domain.DateLayout), Title: "Ringkasan minggu lalu", Body: body, Link: "/app",
	}, true, nil
}

// ---- Inbox and preferences ----------------------------------------------------

type Inbox struct {
	Items  []domain.Notification `json:"items"`
	Unread int                   `json:"unread"`
}

func (s *NotificationService) Inbox(ctx context.Context, userID int64) (*Inbox, error) {
	var out Inbox
	err := parallel(
		func() (e error) { out.Items, e = s.repo.List(ctx, userID, 30); return },
		func() (e error) { out.Unread, e = s.repo.UnreadCount(ctx, userID); return },
	)
	return &out, err
}

func (s *NotificationService) UnreadCount(ctx context.Context, userID int64) (int, error) {
	return s.repo.UnreadCount(ctx, userID)
}

func (s *NotificationService) MarkRead(ctx context.Context, userID int64, ids []int64, all bool) error {
	if all {
		return s.repo.MarkAllRead(ctx, userID, s.clock.Now())
	}
	if len(ids) > 100 {
		return domain.Invalid("ids", "Terlalu banyak sekaligus.")
	}
	return s.repo.MarkRead(ctx, userID, ids, s.clock.Now())
}

// PrefsView is the settings page's shape: every kind with its on/off state.
type PrefsView struct {
	Kinds               map[domain.NotificationKind]bool `json:"kinds"`
	LowBalanceThreshold int64                            `json:"lowBalanceThreshold"`
	ReminderHour        int                              `json:"reminderHour"`
}

func (s *NotificationService) Prefs(ctx context.Context, userID int64) (*PrefsView, error) {
	p, err := s.repo.Prefs(ctx, userID)
	if err != nil {
		return nil, err
	}
	v := &PrefsView{Kinds: map[domain.NotificationKind]bool{}, LowBalanceThreshold: p.LowBalanceThreshold, ReminderHour: p.ReminderHour}
	for _, k := range domain.AllNotificationKinds {
		v.Kinds[k] = p.Enabled(k)
	}
	return v, nil
}

func (s *NotificationService) SavePrefs(ctx context.Context, userID int64, in PrefsView) (*PrefsView, error) {
	if in.LowBalanceThreshold < 0 || in.LowBalanceThreshold > MaxAmount {
		return nil, domain.Invalid("lowBalanceThreshold", "Batas saldo tidak valid.")
	}
	if in.ReminderHour < 0 || in.ReminderHour > 23 {
		return nil, domain.Invalid("reminderHour", "Jam pengingat 0 sampai 23.")
	}
	p := domain.NotificationPrefs{Disabled: map[domain.NotificationKind]bool{}, LowBalanceThreshold: in.LowBalanceThreshold, ReminderHour: in.ReminderHour}
	for k, on := range in.Kinds {
		if !k.Valid() {
			return nil, domain.Invalid("kinds", "Jenis notifikasi tidak dikenal: "+string(k))
		}
		if !on {
			p.Disabled[k] = true
		}
	}
	if err := s.repo.SavePrefs(ctx, userID, p); err != nil {
		return nil, err
	}
	return s.Prefs(ctx, userID)
}

// ChangeNotifiers fans one change out to several listeners (the dashboard
// cache version and the notification worker).
type ChangeNotifiers []domain.ChangeNotifier

// Changed is on the pointer so listeners can be appended after the services
// holding it were built (the notification service needs them first).
func (m *ChangeNotifiers) Changed(userID int64) {
	for _, n := range *m {
		n.Changed(userID)
	}
}
