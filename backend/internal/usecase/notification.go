package usecase

import (
	"context"
	"log"
	"sync"
	"time"

	"receh/internal/domain"
)

// NotificationSource is implemented by modules that have something to tell a
// user. The core handles storage, dedup (ref keys), preferences and delivery.
type NotificationSource interface {
	NotificationKinds() []domain.NotificationKindDef
	NotificationSettings() []domain.NotificationSetting
	// Evaluate emits what applies to one user now. timeDriven is true on the
	// scheduled sweep (reminders that depend on the clock, not on a change).
	Evaluate(ctx context.Context, userID int64, timeDriven bool, prefs domain.NotificationPrefs, emit func(domain.Notification)) error
}

// NotificationService writes in-app reminders. Two things drive it:
//
//   - Changed(userID), called after every write (it is a ChangeNotifier), puts
//     the user on a channel. A worker goroutine re-checks that user's
//     event-driven reminders right away (a budget warning appears just after
//     the expense that caused it).
//   - RunScheduler sweeps every active user on a ticker with a small goroutine
//     pool, adding the time-driven ones (due tomorrow, evening, weekly).
//
// Every notification carries a ref key that is unique per user in the
// database, so evaluating twice, or after a restart, never notifies twice.
type NotificationService struct {
	repo    domain.NotificationRepository
	users   domain.UserRepository
	orgs    OrgMembers
	clock   domain.Clock
	sources []NotificationSource

	queue   chan int64 // workspace ids
	mu      sync.Mutex
	pending map[int64]bool // workspaces already queued, so bursts of writes coalesce
	workers int
	runMu   sync.Mutex // one sweep at a time
}

// OrgMembers lists who belongs to a workspace.
type OrgMembers interface {
	MemberIDs(ctx context.Context, orgID int64) ([]int64, error)
}

func NewNotificationService(r domain.NotificationRepository, u domain.UserRepository, o OrgMembers, clock domain.Clock) *NotificationService {
	return &NotificationService{
		repo: r, users: u, orgs: o, clock: clock,
		queue: make(chan int64, 1024), pending: map[int64]bool{}, workers: 4,
	}
}

// AddSource registers a module's notifications (call before serving).
func (s *NotificationService) AddSource(src NotificationSource) { s.sources = append(s.sources, src) }

// Changed implements domain.ChangeNotifier: data of a workspace changed, so
// every member gets re-checked. It never blocks a request: when the queue is
// full the check is skipped and the next sweep catches up.
func (s *NotificationService) Changed(orgID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending[orgID] {
		return
	}
	select {
	case s.queue <- orgID:
		s.pending[orgID] = true
	default:
	}
}

// RunWorker processes queued users until ctx ends. Run it in its own goroutine.
func (s *NotificationService) RunWorker(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case orgID := <-s.queue:
			s.mu.Lock()
			delete(s.pending, orgID)
			s.mu.Unlock()
			ids, err := s.orgs.MemberIDs(ctx, orgID)
			if err != nil && ctx.Err() == nil {
				log.Printf("notifications: org %d members: %v", orgID, err)
			}
			for _, uid := range ids {
				if _, err := s.Evaluate(ctx, uid, false); err != nil && ctx.Err() == nil {
					log.Printf("notifications: user %d: %v", uid, err)
				}
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

// Evaluate asks every source about one user and stores what is new and enabled.
func (s *NotificationService) Evaluate(ctx context.Context, userID int64, timeDriven bool) (int, error) {
	prefs, err := s.repo.Prefs(ctx, userID)
	if err != nil {
		return 0, err
	}
	var all []domain.Notification
	emit := func(n domain.Notification) {
		if prefs.Enabled(n.Kind) {
			n.UserID = userID
			all = append(all, n)
		}
	}
	for _, src := range s.sources {
		if err := src.Evaluate(ctx, userID, timeDriven, prefs, emit); err != nil {
			return 0, err
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

// ---- Inbox and preferences ----------------------------------------------------

type Inbox struct {
	Items  []domain.Notification `json:"items"`
	Unread int                   `json:"unread"`
	// Kinds describes every registered kind (icon, tone, label) for display.
	Kinds []domain.NotificationKindDef `json:"kinds"`
}

func (s *NotificationService) Inbox(ctx context.Context, userID int64) (*Inbox, error) {
	out := Inbox{Kinds: s.kinds()}
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

// kinds lists every kind registered by the sources.
func (s *NotificationService) kinds() []domain.NotificationKindDef {
	out := []domain.NotificationKindDef{}
	for _, src := range s.sources {
		out = append(out, src.NotificationKinds()...)
	}
	return out
}

// KindPref is one kind with its definition and on/off state.
type KindPref struct {
	domain.NotificationKindDef
	Enabled bool `json:"enabled"`
}

// SettingPref is one module setting with its current value.
type SettingPref struct {
	domain.NotificationSetting
	Value int64 `json:"value"`
}

// PrefsView is the settings page: every registered kind and setting.
type PrefsView struct {
	Kinds    []KindPref    `json:"kinds"`
	Settings []SettingPref `json:"settings"`
}

// PrefsInput is what the settings page sends back.
type PrefsInput struct {
	Kinds    map[domain.NotificationKind]bool `json:"kinds"`
	Settings map[string]int64                 `json:"settings"`
}

func (s *NotificationService) Prefs(ctx context.Context, userID int64) (*PrefsView, error) {
	p, err := s.repo.Prefs(ctx, userID)
	if err != nil {
		return nil, err
	}
	v := &PrefsView{Kinds: []KindPref{}, Settings: []SettingPref{}}
	for _, src := range s.sources {
		for _, k := range src.NotificationKinds() {
			v.Kinds = append(v.Kinds, KindPref{NotificationKindDef: k, Enabled: p.Enabled(k.Key)})
		}
		for _, st := range src.NotificationSettings() {
			v.Settings = append(v.Settings, SettingPref{NotificationSetting: st, Value: p.Setting(st.Key, st.Default)})
		}
	}
	return v, nil
}

func (s *NotificationService) SavePrefs(ctx context.Context, userID int64, in PrefsInput) (*PrefsView, error) {
	kinds := map[domain.NotificationKind]bool{}
	settings := map[string]domain.NotificationSetting{}
	for _, src := range s.sources {
		for _, k := range src.NotificationKinds() {
			kinds[k.Key] = true
		}
		for _, st := range src.NotificationSettings() {
			settings[st.Key] = st
		}
	}
	p := domain.NotificationPrefs{Disabled: map[domain.NotificationKind]bool{}, Settings: map[string]int64{}}
	for k, on := range in.Kinds {
		if !kinds[k] {
			return nil, domain.Invalid("kinds", "Jenis notifikasi tidak dikenal: "+string(k))
		}
		if !on {
			p.Disabled[k] = true
		}
	}
	for key, val := range in.Settings {
		st, ok := settings[key]
		if !ok {
			return nil, domain.Invalid("settings", "Pengaturan tidak dikenal: "+key)
		}
		if val < st.Min || val > st.Max {
			return nil, domain.Invalid(key, "Nilai di luar batas yang diizinkan.")
		}
		p.Settings[key] = val
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
