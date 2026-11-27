package usecase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/ratelimit"
)

// BillingOrgs is the part of OrgService billing needs.
type BillingOrgs interface {
	OwnerOf(ctx context.Context, orgID int64) (int64, error)
	Org(ctx context.Context, orgID int64) (*domain.Org, error)
}

// MockGateway is implemented by the mock provider: it lets a person on the
// fake checkout page choose the outcome.
type MockGateway interface {
	Complete(ref, outcome string) (externalID string, err error)
}

// IntegrationStatus tells the admin which gateway settings are filled in. It
// carries yes/no only, never a key.
type IntegrationStatus struct {
	Provider   string      `json:"provider"`
	Mode       string      `json:"mode"` // sandbox | production | local
	Keys       []KeyStatus `json:"keys"`
	WebhookURL string      `json:"webhookUrl"`
	ReturnURL  string      `json:"returnUrl"`
}

type KeyStatus struct {
	Name string `json:"name"`
	Set  bool   `json:"set"`
}

type BillingConfig struct {
	Enabled   bool
	Catalog   domain.Catalog
	PublicURL string
	Provider  domain.PaymentProvider
	Status    IntegrationStatus
	// Defs returns the plan limits modules registered (read at request time,
	// because modules register after the service is built).
	Defs func() []domain.LimitDef
}

// BillingService sells plans: it opens checkouts, learns from the gateway
// that they were paid, and keeps each workspace's subscription current.
//
// The gateway is never trusted on its word. A webhook only says "payment X
// changed"; the service then asks the gateway directly (Fetch) and checks the
// amount before it grants anything.
type BillingService struct {
	cfg    BillingConfig
	repo   domain.BillingRepository
	orgs   BillingOrgs
	users  domain.UserRepository
	audit  *AuditRecorder
	clock  domain.Clock
	checks *ratelimit.Limiter // checkouts per user
	syncs  *ratelimit.Limiter // manual status checks per user
}

func NewBillingService(cfg BillingConfig, r domain.BillingRepository, o BillingOrgs, u domain.UserRepository,
	a *AuditRecorder, clock domain.Clock) *BillingService {
	return &BillingService{
		cfg: cfg, repo: r, orgs: o, users: u, audit: a, clock: clock,
		checks: ratelimit.New(10, time.Hour), syncs: ratelimit.New(30, time.Hour),
	}
}

func (s *BillingService) Enabled() bool { return s.cfg.Enabled && s.cfg.Provider != nil }

// ---- Reading ------------------------------------------------------------------------

// CatalogView is what the public pricing page shows.
type CatalogView struct {
	Enabled  bool           `json:"enabled"`
	Provider string         `json:"provider,omitempty"`
	Catalog  domain.Catalog `json:"catalog"`
	// Defs lists what each plan includes (modules register them), free and Pro side by side.
	Defs []domain.LimitDef `json:"defs"`
}

func (s *BillingService) PublicCatalog() CatalogView {
	if !s.Enabled() {
		return CatalogView{Defs: []domain.LimitDef{}}
	}
	defs := []domain.LimitDef{}
	if s.cfg.Defs != nil {
		defs = s.cfg.Defs()
	}
	return CatalogView{Enabled: true, Provider: s.cfg.Provider.Name(), Catalog: s.cfg.Catalog, Defs: defs}
}

type SubscriptionView struct {
	State             domain.SubState `json:"state"`
	PriceID           string          `json:"priceId"`
	PeriodEnd         time.Time       `json:"periodEnd"`
	GraceEndsAt       time.Time       `json:"graceEndsAt"`
	CancelAtPeriodEnd bool            `json:"cancelAtPeriodEnd"`
}

type BillingOverview struct {
	Enabled      bool              `json:"enabled"`
	IsOwner      bool              `json:"isOwner"`
	Provider     string            `json:"provider,omitempty"`
	Subscription *SubscriptionView `json:"subscription"`
	// Pending and Payments are only for the owner: they are the owner's money.
	Pending  *domain.Payment  `json:"pending"`
	Payments []domain.Payment `json:"payments"`
}

func (s *BillingService) Overview(ctx context.Context, u *domain.User, orgID int64) (*BillingOverview, error) {
	out := &BillingOverview{Payments: []domain.Payment{}}
	if !s.Enabled() {
		return out, nil
	}
	owner, err := s.orgs.OwnerOf(ctx, orgID)
	if err != nil {
		return nil, err
	}
	out.Enabled, out.Provider, out.IsOwner = true, s.cfg.Provider.Name(), owner == u.ID
	now := s.clock.Now()
	sub, err := s.repo.SubscriptionOf(ctx, orgID)
	if err != nil && !errors.Is(err, domain.ErrNoRows) {
		return nil, err
	}
	if sub != nil {
		out.Subscription = &SubscriptionView{
			State: sub.State(now, s.cfg.Catalog.Grace()), PriceID: sub.PriceID, PeriodEnd: sub.PeriodEnd,
			GraceEndsAt: sub.PeriodEnd.Add(s.cfg.Catalog.Grace()), CancelAtPeriodEnd: sub.CancelAtPeriodEnd,
		}
	}
	if !out.IsOwner {
		return out, nil
	}
	if p, err := s.repo.PendingFor(ctx, orgID, now); err == nil {
		out.Pending = p
	} else if !errors.Is(err, domain.ErrNoRows) {
		return nil, err
	}
	if out.Payments, err = s.repo.PaymentsForOrg(ctx, orgID, 20); err != nil {
		return nil, err
	}
	return out, nil
}

// ---- Checkout --------------------------------------------------------------------------

func (s *BillingService) requireOwner(ctx context.Context, u *domain.User, orgID int64) error {
	if !s.Enabled() {
		return domain.BillingOff()
	}
	owner, err := s.orgs.OwnerOf(ctx, orgID)
	if err != nil {
		return err
	}
	if owner != u.ID {
		return domain.Forbidden("OWNER_ONLY", "Hanya owner workspace yang bisa mengelola langganan.")
	}
	return nil
}

func newExternalID(orgID int64) (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("rc-%d-%s", orgID, hex.EncodeToString(b)), nil
}

// Checkout opens (or reuses) the workspace's one payable checkout for a price.
func (s *BillingService) Checkout(ctx context.Context, u *domain.User, orgID int64, priceID string, client domain.ClientInfo) (*domain.Payment, error) {
	if err := s.requireOwner(ctx, u, orgID); err != nil {
		return nil, err
	}
	price, ok := s.cfg.Catalog.Price(priceID)
	if !ok {
		return nil, domain.Invalid("priceId", "Paket tidak dikenal.")
	}
	key := "u" + itoa64(u.ID)
	if s.checks.Blocked(key) {
		return nil, domain.TooMany("Terlalu banyak percobaan pembayaran. Coba lagi nanti.")
	}
	s.checks.Hit(key)
	org, err := s.orgs.Org(ctx, orgID)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	if pend, err := s.repo.PendingFor(ctx, orgID, now); err == nil {
		if pend.PriceID == price.ID && pend.CheckoutURL != "" {
			return pend, nil
		}
		s.expire(ctx, pend) // A different choice replaces the old checkout.
	} else if !errors.Is(err, domain.ErrNoRows) {
		return nil, err
	}
	ext, err := newExternalID(orgID)
	if err != nil {
		return nil, err
	}
	p := &domain.Payment{
		OrgID: orgID, OrgName: org.Name, PayerID: &u.ID, PayerEmail: u.Email, PriceID: price.ID,
		Amount: price.Amount, Currency: s.cfg.Catalog.Currency, Provider: s.cfg.Provider.Name(),
		ExternalID: ext, ExpiresAt: now.Add(s.cfg.Catalog.InvoiceTTL),
	}
	cur, created, err := s.repo.Reserve(ctx, p, now)
	if err != nil {
		return nil, notFoundOr(err, "Workspace tidak ditemukan.")
	}
	if !created {
		if cur.CheckoutURL == "" {
			return nil, domain.Conflict("CHECKOUT_BUSY", "Pembayaran sedang disiapkan. Coba lagi sebentar.")
		}
		return cur, nil
	}
	co, err := s.cfg.Provider.CreateCheckout(ctx, domain.CheckoutRequest{
		ExternalID: ext, Amount: price.Amount, Currency: s.cfg.Catalog.Currency,
		Description: price.Label["id"] + " · " + org.Name, PayerEmail: u.Email, PayerName: u.Name,
		ExpiresAt: p.ExpiresAt, ReturnURL: s.cfg.PublicURL + "/app/langganan?status=return",
	})
	if err != nil {
		log.Printf("billing: checkout for org %d: %v", orgID, err)
		_ = s.repo.Fail(ctx, p.ID)
		return nil, &domain.AppError{Kind: domain.KindUnavailable, Code: "GATEWAY_ERROR",
			Message: "Layanan pembayaran sedang bermasalah. Coba lagi beberapa saat lagi."}
	}
	if err := s.repo.SetCheckout(ctx, p.ID, co.ProviderRef, co.URL); err != nil {
		return nil, err
	}
	p.ProviderRef, p.CheckoutURL = co.ProviderRef, co.URL
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventPaymentCreated, Client: client,
		Detail: price.ID + " · " + org.Name + " · " + s.cfg.Catalog.Currency + " " + itoa64(price.Amount)})
	return p, nil
}

// expire stops a pending checkout. If the gateway says it was in fact paid,
// that wins: the money is on record and the period is granted.
func (s *BillingService) expire(ctx context.Context, p *domain.Payment) {
	if res, err := s.reconcile(ctx, p, ""); err == nil && res != nil && res.Activated {
		return
	}
	if err := s.cfg.Provider.Cancel(ctx, p.ExternalID, p.ProviderRef); err != nil {
		log.Printf("billing: cancel payment %d: %v", p.ID, err)
	}
	if _, err := s.repo.Settle(ctx, domain.SettleInput{PaymentID: p.ID, Status: domain.PaymentExpired, Now: s.clock.Now()}); err != nil {
		log.Printf("billing: expire payment %d: %v", p.ID, err)
		return
	}
	s.audit.Record(AuditEntry{Email: p.PayerEmail, Event: domain.EventPaymentExpired, Detail: "Checkout " + p.ExternalID})
}

// CancelPayment lets the owner drop a checkout they no longer want.
func (s *BillingService) CancelPayment(ctx context.Context, u *domain.User, orgID, paymentID int64) error {
	if err := s.requireOwner(ctx, u, orgID); err != nil {
		return err
	}
	p, err := s.paymentOf(ctx, orgID, paymentID)
	if err != nil {
		return err
	}
	if p.Status != domain.PaymentPending {
		return domain.Conflict("PAYMENT_FINISHED", "Pembayaran ini sudah selesai.")
	}
	s.expire(ctx, p)
	return nil
}

func (s *BillingService) paymentOf(ctx context.Context, orgID, paymentID int64) (*domain.Payment, error) {
	p, err := s.repo.PaymentByID(ctx, paymentID)
	if err != nil || p.OrgID != orgID {
		return nil, domain.NotFound("Pembayaran tidak ditemukan.")
	}
	return p, nil
}

// SyncPayment asks the gateway about a payment now (a lost webhook is fine).
func (s *BillingService) SyncPayment(ctx context.Context, u *domain.User, orgID, paymentID int64) (*domain.Payment, error) {
	if err := s.requireOwner(ctx, u, orgID); err != nil {
		return nil, err
	}
	key := "u" + itoa64(u.ID)
	if s.syncs.Blocked(key) {
		return nil, domain.TooMany("Terlalu sering memeriksa. Coba lagi nanti.")
	}
	s.syncs.Hit(key)
	p, err := s.paymentOf(ctx, orgID, paymentID)
	if err != nil {
		return nil, err
	}
	return s.syncAndReload(ctx, p)
}

// AdminSyncPayment is the same for the admin, over any payment.
func (s *BillingService) AdminSyncPayment(ctx context.Context, paymentID int64) (*domain.Payment, error) {
	if !s.Enabled() {
		return nil, domain.BillingOff()
	}
	p, err := s.repo.PaymentByID(ctx, paymentID)
	if err != nil {
		return nil, notFoundOr(err, "Pembayaran tidak ditemukan.")
	}
	return s.syncAndReload(ctx, p)
}

func (s *BillingService) syncAndReload(ctx context.Context, p *domain.Payment) (*domain.Payment, error) {
	if p.Status == domain.PaymentPending {
		if _, err := s.reconcile(ctx, p, ""); err != nil {
			log.Printf("billing: sync payment %d: %v", p.ID, err)
			return nil, &domain.AppError{Kind: domain.KindUnavailable, Code: "GATEWAY_ERROR",
				Message: "Belum bisa menghubungi layanan pembayaran. Coba lagi nanti."}
		}
	}
	return s.repo.PaymentByID(ctx, p.ID)
}

// SetRenewal turns "stop renewing" on or off. The paid period keeps running
// either way; this only decides whether reminders ask for renewal.
func (s *BillingService) SetRenewal(ctx context.Context, u *domain.User, orgID int64, stop bool, client domain.ClientInfo) error {
	if err := s.requireOwner(ctx, u, orgID); err != nil {
		return err
	}
	ok, err := s.repo.SetCancelAtPeriodEnd(ctx, orgID, stop)
	if err != nil {
		return err
	}
	if !ok {
		return domain.Conflict("NO_SUBSCRIPTION", "Workspace ini belum berlangganan.")
	}
	ev, text := domain.EventSubscriptionResumed, "Lanjut perpanjangan"
	if stop {
		ev, text = domain.EventSubscriptionStopped, "Berhenti perpanjangan"
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: ev, Client: client, Detail: text + " · workspace " + itoa64(orgID)})
	return nil
}

// ---- What the gateway says --------------------------------------------------------------

// HandleWebhook processes a gateway notification. Authentic but unknown
// payments are ignored (so the gateway stops retrying); a bad signature is
// domain.ErrBadSignature; a transient failure returns an error so the gateway
// delivers again.
func (s *BillingService) HandleWebhook(ctx context.Context, provider string, raw []byte, header func(string) string) error {
	if !s.Enabled() || provider != s.cfg.Provider.Name() {
		return domain.NotFound("")
	}
	ev, err := s.cfg.Provider.ParseWebhook(raw, header)
	if err != nil {
		return err
	}
	p, err := s.repo.PaymentByExternalID(ctx, ev.ExternalID)
	if errors.Is(err, domain.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = s.reconcile(ctx, p, ev.EventID)
	return err
}

// MockComplete is the mock checkout page's "pay" button. It records the
// outcome in the mock, then delivers the same webhook a real gateway would.
func (s *BillingService) MockComplete(ctx context.Context, ref, outcome string) error {
	m, ok := s.cfg.Provider.(MockGateway)
	if !s.Enabled() || !ok {
		return domain.NotFound("")
	}
	ext, err := m.Complete(ref, outcome)
	if err != nil {
		return domain.Invalid("", "Pembayaran tiruan ini tidak bisa diproses: "+err.Error())
	}
	raw, _ := json.Marshal(map[string]string{"eventId": ref + ":" + outcome, "externalId": ext})
	return s.HandleWebhook(ctx, s.cfg.Provider.Name(), raw, func(string) string { return "" })
}

// reconcile asks the gateway about p and applies the answer. A nil result with
// a nil error means "still pending, nothing to apply".
func (s *BillingService) reconcile(ctx context.Context, p *domain.Payment, eventID string) (*domain.SettleResult, error) {
	st, err := s.cfg.Provider.Fetch(ctx, p.ExternalID, p.ProviderRef)
	if err != nil {
		return nil, err
	}
	if st.Status == domain.PaymentPending {
		return nil, nil
	}
	price, ok := s.cfg.Catalog.Price(p.PriceID)
	if !ok {
		return nil, fmt.Errorf("payment %d: price %q is no longer in the catalog", p.ID, p.PriceID)
	}
	if st.Status == domain.PaymentPaid && st.Amount != p.Amount {
		s.audit.Record(AuditEntry{Email: p.PayerEmail, Event: domain.EventPaymentMismatch,
			Detail: fmt.Sprintf("Checkout %s: dibayar %d, seharusnya %d", p.ExternalID, st.Amount, p.Amount)})
		log.Printf("billing: payment %d amount mismatch: gateway %d, expected %d", p.ID, st.Amount, p.Amount)
		return nil, nil
	}
	now := s.clock.Now()
	paidAt := now
	if st.PaidAt != nil {
		paidAt = *st.PaidAt
	}
	res, err := s.repo.Settle(ctx, domain.SettleInput{
		PaymentID: p.ID, Provider: s.cfg.Provider.Name(), EventID: eventID, Status: st.Status, PaidAt: paidAt,
		Price: price, Grace: s.cfg.Catalog.Grace(), Now: now, Note: price.Label["id"],
	})
	if err != nil {
		return nil, err
	}
	if res.Activated {
		s.audit.Record(AuditEntry{UserID: p.PayerID, Email: p.PayerEmail, Event: domain.EventPaymentPaid,
			Detail: price.ID + " · " + p.OrgName + " · sampai " + res.Payment.PeriodEnd.Format(domain.DateLayout)})
	}
	return res, nil
}

// ---- Background sweep ---------------------------------------------------------------------

// Sweep catches what webhooks missed: it asks the gateway about checkouts that
// are still pending, expires the ones past their time, and stores the derived
// state of subscriptions. Granting never depends on it (an entitlement lapses
// by itself), so a late or skipped sweep only delays tidiness.
func (s *BillingService) Sweep(ctx context.Context) error {
	if !s.Enabled() {
		return nil
	}
	now := s.clock.Now()
	pending, err := s.repo.PendingBefore(ctx, now.Add(-5*time.Minute), 100)
	if err != nil {
		return err
	}
	for i := range pending {
		p := &pending[i]
		res, err := s.reconcile(ctx, p, "")
		if err != nil {
			log.Printf("billing: sweep payment %d: %v", p.ID, err)
		}
		if res == nil && !now.Before(p.ExpiresAt) {
			s.expire(ctx, p)
		}
	}
	return s.repo.SyncSubscriptionStates(ctx, now, s.cfg.Catalog.Grace())
}

// RunSweeper runs Sweep on a ticker until ctx ends.
func (s *BillingService) RunSweeper(ctx context.Context, every time.Duration) {
	if !s.Enabled() {
		return
	}
	run := func() {
		if err := s.Sweep(ctx); err != nil && ctx.Err() == nil {
			log.Printf("billing: sweep: %v", err)
		}
	}
	run()
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}

// ---- Admin ---------------------------------------------------------------------------------

type PaymentPage struct {
	Items []domain.Payment `json:"items"`
	Total int              `json:"total"`
}

func (s *BillingService) AdminPayments(ctx context.Context, status domain.PaymentStatus, page int) (*PaymentPage, error) {
	if page < 1 {
		page = 1
	}
	const size = 25
	items, total, err := s.repo.PaymentsAll(ctx, status, size, (page-1)*size)
	if err != nil {
		return nil, err
	}
	return &PaymentPage{Items: items, Total: total}, nil
}

func (s *BillingService) IntegrationStatus() IntegrationStatus {
	st := s.cfg.Status
	if st.Keys == nil {
		st.Keys = []KeyStatus{}
	}
	return st
}

// ---- Notifications (the owner is reminded to renew) -----------------------------------------

const NotifBilling domain.NotificationKind = "billing"

func (s *BillingService) NotificationKinds() []domain.NotificationKindDef {
	if !s.Enabled() {
		return nil
	}
	return []domain.NotificationKindDef{{
		Key: NotifBilling, Icon: "sparkle", Tone: "warn",
		Label: map[string]string{"id": "Langganan dan pembayaran", "en": "Subscription and billing"},
	}}
}

func (s *BillingService) NotificationSettings() []domain.NotificationSetting { return nil }

// Evaluate reminds the owner before a paid period ends, when it is in its
// grace days, and once it has ended. Ref keys include the period end, so each
// reminder is sent once per period.
func (s *BillingService) Evaluate(ctx context.Context, userID int64, timeDriven bool, _ domain.NotificationPrefs, emit func(domain.Notification)) error {
	if !s.Enabled() || !timeDriven {
		return nil
	}
	subs, err := s.repo.OwnedSubscriptions(ctx, userID)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	grace := s.cfg.Catalog.Grace()
	notice := time.Duration(s.cfg.Catalog.RenewalNoticeDays) * 24 * time.Hour
	day := func(t time.Time) string { return t.In(now.Location()).Format(domain.DateLayout) }
	for _, sub := range subs {
		prefix := ""
		if !sub.Personal {
			prefix = "[" + sub.OrgName + "] "
		}
		ref := fmt.Sprintf("%d:%d", sub.OrgID, sub.PeriodEnd.Unix())
		n := domain.Notification{Kind: NotifBilling, Link: "/app/langganan"}
		switch sub.State(now, grace) {
		case domain.SubActive:
			if sub.PeriodEnd.Sub(now) > notice {
				continue
			}
			n.Title, n.RefKey = prefix+"Langganan Pro segera berakhir", "billing:renew:"+ref
			n.Body = "Aktif sampai " + day(sub.PeriodEnd) + ". Perpanjang agar tidak turun ke paket Gratis."
		case domain.SubGrace:
			n.Title, n.RefKey = prefix+"Langganan Pro sudah berakhir", "billing:grace:"+ref
			n.Body = "Masa tenggang sampai " + day(sub.PeriodEnd.Add(grace)) + ". Perpanjang sekarang agar fiturnya tetap terbuka."
		case domain.SubExpired:
			if now.After(sub.PeriodEnd.Add(grace).AddDate(0, 0, 30)) {
				continue
			}
			n.Title, n.RefKey = prefix+"Workspace kembali ke paket Gratis", "billing:expired:"+ref
			n.Body = "Langganan berakhir " + day(sub.PeriodEnd) + ". Data tetap aman; berlangganan lagi kapan saja."
		default:
			continue
		}
		emit(n)
	}
	return nil
}
