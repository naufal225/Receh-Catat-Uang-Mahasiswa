package usecase

import (
	"context"
	"crypto/rand"
	"errors"
	"regexp"
	"strings"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/ratelimit"
)

// MembershipService decides what a user may use. It is the single place that
// answers "is this user premium?", so every limit is enforced the same way.
//
// With billing disabled (the personal edition) everyone is premium and no
// purchase or code is ever needed.
type MembershipService struct {
	billing bool
	ents    domain.EntitlementRepository
	promos  domain.PromoRepository
	users   domain.UserRepository
	audit   *AuditRecorder
	// orgs says who owns a workspace: a workspace has the plan of its owner.
	orgs  OrgOwners
	clock domain.Clock
	// Guessing codes is the obvious abuse; a few tries per hour is plenty.
	redeemLimit *ratelimit.Limiter
	// Limits and usage counters registered by modules.
	defs     []domain.LimitDef
	counters []func(ctx context.Context, orgID int64, u domain.Usage) error
}

func NewMembershipService(billing bool, e domain.EntitlementRepository, p domain.PromoRepository, u domain.UserRepository,
	o OrgOwners, a *AuditRecorder, clock domain.Clock) *MembershipService {
	return &MembershipService{
		billing: billing, ents: e, promos: p, users: u, orgs: o, audit: a, clock: clock,
		redeemLimit: ratelimit.New(10, time.Hour),
	}
}

// OrgOwners is the part of OrgService membership needs.
type OrgOwners interface {
	OwnerOf(ctx context.Context, orgID int64) (int64, error)
}

func (s *MembershipService) BillingEnabled() bool { return s.billing }

// AddLimits registers limits a module enforces (call before serving).
func (s *MembershipService) AddLimits(defs ...domain.LimitDef) { s.defs = append(s.defs, defs...) }

// Defs returns a copy of the registered limits.
func (s *MembershipService) Defs() []domain.LimitDef { return append([]domain.LimitDef{}, s.defs...) }

// AddUsageCounter registers how a module counts what a user has, for display.
func (s *MembershipService) AddUsageCounter(fn func(ctx context.Context, orgID int64, u domain.Usage) error) {
	s.counters = append(s.counters, fn)
}

// Plan returns the plan of a workspace and, when premium comes from an
// entitlement, it. Account-wide entitlements of the owner (comps, codes) and
// payments for this workspace all count; the one lasting longest wins.
func (s *MembershipService) Plan(ctx context.Context, ownerID, orgID int64) (domain.Plan, *domain.Entitlement, error) {
	if !s.billing {
		return domain.PlanPremium, nil, nil
	}
	e, err := s.ents.ActiveFor(ctx, ownerID, orgID, s.clock.Now())
	if errors.Is(err, domain.ErrNoRows) {
		return domain.PlanFree, nil, nil
	}
	if err != nil {
		return "", nil, err
	}
	return e.Plan, e, nil
}

// Limit is what modules call (through kit.PlanLimits) before creating
// something limited. A workspace has the plan of its owner. An unregistered
// key is unlimited.
func (s *MembershipService) Limit(ctx context.Context, orgID int64, key string) (int, error) {
	owner, err := s.orgs.OwnerOf(ctx, orgID)
	if err != nil {
		return 0, err
	}
	p, _, err := s.Plan(ctx, owner, orgID)
	if err != nil {
		return 0, err
	}
	if v, ok := domain.LimitsFor(p, s.defs)[key]; ok {
		return v, nil
	}
	return domain.Unlimited, nil
}

// Get describes the plan of a workspace as seen by viewerID.
func (s *MembershipService) Get(ctx context.Context, viewerID, orgID int64) (*domain.Membership, error) {
	owner, err := s.orgs.OwnerOf(ctx, orgID)
	if err != nil {
		return nil, err
	}
	plan, ent, err := s.Plan(ctx, owner, orgID)
	if err != nil {
		return nil, err
	}
	m := &domain.Membership{
		OrgID: orgID, IsOwner: owner == viewerID,
		Plan: plan, BillingEnabled: s.billing,
		Limits: domain.LimitsFor(plan, s.defs), FreeLimits: domain.LimitsFor(domain.PlanFree, s.defs),
		Usage: domain.Usage{}, Defs: append([]domain.LimitDef{}, s.defs...),
	}
	if ent != nil {
		m.Source, m.EndsAt, m.Note = ent.Source, ent.EndsAt, ent.Note
	}
	for _, count := range s.counters {
		if err := count(ctx, orgID, m.Usage); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// ---- Promo codes: redeeming ----------------------------------------------------

var codeRe = regexp.MustCompile(`^[A-Z0-9][A-Z0-9-]{3,31}$`)

func normalizeCode(c string) string { return strings.ToUpper(strings.TrimSpace(c)) }

func (s *MembershipService) Redeem(ctx context.Context, u *domain.User, orgID int64, code string, client domain.ClientInfo) (*domain.Membership, error) {
	if !s.billing {
		return nil, domain.Invalid("code", "Edisi ini sudah membuka semua fitur Premium; kode tidak diperlukan.")
	}
	key := "u" + itoa64(u.ID)
	if s.redeemLimit.Blocked(key) {
		return nil, domain.TooMany("Terlalu banyak percobaan kode. Coba lagi dalam satu jam.")
	}
	code = normalizeCode(code)
	if !codeRe.MatchString(code) {
		s.redeemLimit.Hit(key)
		return nil, &domain.AppError{Kind: domain.KindInvalid, Code: domain.ErrPromoInvalid.Code, Message: domain.ErrPromoInvalid.Message, Field: "code"}
	}
	ent, err := s.promos.Redeem(ctx, code, u.ID, s.clock.Now())
	if err != nil {
		s.redeemLimit.Hit(key)
		var app *domain.AppError
		if errors.As(err, &app) {
			return nil, &domain.AppError{Kind: app.Kind, Code: app.Code, Message: app.Message, Field: "code"}
		}
		return nil, err
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventPromoRedeemed, Client: client,
		Detail: "Kode " + code + untilText(ent.EndsAt)})
	return s.Get(ctx, u.ID, orgID)
}

func untilText(t *time.Time) string {
	if t == nil {
		return " (selamanya)"
	}
	return " (sampai " + t.Format(domain.DateLayout) + ")"
}

// ---- Admin: complimentary premium ------------------------------------------------

// Grant gives a user premium for free, e.g. family. until nil means forever;
// otherwise premium ends at the start of the day after `until`.
func (s *MembershipService) Grant(ctx context.Context, actor *domain.User, userID int64, until string, note string, client domain.ClientInfo) (*domain.Entitlement, error) {
	target, err := s.users.ByID(ctx, userID)
	if err != nil {
		return nil, notFoundOr(err, "Pengguna tidak ditemukan.")
	}
	if target.Role != domain.RoleUser {
		return nil, domain.Invalid("", "Premium hanya untuk akun pengguna, bukan admin.")
	}
	note = strings.Join(strings.Fields(note), " ")
	if len([]rune(note)) > 200 {
		return nil, domain.Invalid("note", "Catatan maksimal 200 karakter.")
	}
	now := s.clock.Now()
	e := &domain.Entitlement{UserID: userID, Source: domain.SourceComp, Plan: domain.PlanPremium, StartsAt: now, Note: note, GrantedBy: &actor.ID}
	if until != "" {
		d, err := domain.ParseDate(until)
		if err != nil {
			return nil, domain.Invalid("until", "Tanggal tidak valid.")
		}
		end := time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, now.Location())
		if !end.After(now) {
			return nil, domain.Invalid("until", "Tanggal berakhir harus hari ini atau setelahnya.")
		}
		e.EndsAt = &end
	}
	if err := s.ents.Create(ctx, e); err != nil {
		return nil, err
	}
	s.audit.Record(AuditEntry{UserID: &actor.ID, Email: actor.Email, Event: domain.EventPremiumGranted, Client: client,
		Detail: "Premium gratis untuk " + target.Email + untilText(e.EndsAt) + noteSuffix(note)})
	return e, nil
}

func noteSuffix(note string) string {
	if note == "" {
		return ""
	}
	return " · " + note
}

func (s *MembershipService) Revoke(ctx context.Context, actor *domain.User, entitlementID int64, client domain.ClientInfo) error {
	e, err := s.ents.ByID(ctx, entitlementID)
	if err != nil {
		return notFoundOr(err, "Data Premium tidak ditemukan.")
	}
	ok, err := s.ents.Revoke(ctx, entitlementID, s.clock.Now())
	if err != nil {
		return err
	}
	if !ok {
		return domain.Conflict("ALREADY_REVOKED", "Premium ini sudah dicabut sebelumnya.")
	}
	email := ""
	if u, err := s.users.ByID(ctx, e.UserID); err == nil {
		email = u.Email
	}
	s.audit.Record(AuditEntry{UserID: &actor.ID, Email: actor.Email, Event: domain.EventPremiumRevoked, Client: client,
		Detail: "Premium " + string(e.Source) + " dicabut dari " + email})
	return nil
}

func (s *MembershipService) ListForUser(ctx context.Context, userID int64) ([]domain.Entitlement, error) {
	if _, err := s.users.ByID(ctx, userID); err != nil {
		return nil, notFoundOr(err, "Pengguna tidak ditemukan.")
	}
	return s.ents.ListForUser(ctx, userID)
}

// MarkPlans fills Plan on a page of users for the admin list.
func (s *MembershipService) MarkPlans(ctx context.Context, users []domain.User) error {
	ids := make([]int64, 0, len(users))
	for _, u := range users {
		if u.Role == domain.RoleUser {
			ids = append(ids, u.ID)
		}
	}
	premium := map[int64]bool{}
	if s.billing {
		var err error
		if premium, err = s.ents.PremiumUserIDs(ctx, ids, s.clock.Now()); err != nil {
			return err
		}
	}
	for i := range users {
		if users[i].Role != domain.RoleUser {
			continue
		}
		users[i].Plan = domain.PlanFree
		if !s.billing || premium[users[i].ID] {
			users[i].Plan = domain.PlanPremium
		}
	}
	return nil
}

// ---- Admin: promo codes ------------------------------------------------------------

type PromoInput struct {
	Code         string `json:"code"`
	DurationDays *int   `json:"durationDays"`
	MaxUses      int    `json:"maxUses"`
	ExpiresOn    string `json:"expiresOn"`
	Note         string `json:"note"`
}

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // No 0/O or 1/I to misread.

func randomCode() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	out := []byte("RECEH-")
	for _, x := range b {
		out = append(out, codeAlphabet[int(x)%len(codeAlphabet)])
	}
	return string(out), nil
}

func (s *MembershipService) CreatePromo(ctx context.Context, actor *domain.User, in PromoInput, client domain.ClientInfo) (*domain.PromoCode, error) {
	code := normalizeCode(in.Code)
	if code == "" {
		var err error
		if code, err = randomCode(); err != nil {
			return nil, err
		}
	} else if !codeRe.MatchString(code) {
		return nil, domain.Invalid("code", "Kode 4–32 karakter: huruf, angka, dan tanda hubung.")
	}
	if in.MaxUses < 1 || in.MaxUses > 10_000 {
		return nil, domain.Invalid("maxUses", "Kuota 1 sampai 10.000 pemakaian.")
	}
	if in.DurationDays != nil && (*in.DurationDays < 1 || *in.DurationDays > 3650) {
		return nil, domain.Invalid("durationDays", "Durasi 1 sampai 3.650 hari, atau kosongkan untuk selamanya.")
	}
	note := strings.Join(strings.Fields(in.Note), " ")
	if len([]rune(note)) > 200 {
		return nil, domain.Invalid("note", "Catatan maksimal 200 karakter.")
	}
	p := &domain.PromoCode{Code: code, Plan: domain.PlanPremium, DurationDays: in.DurationDays, MaxUses: in.MaxUses, Note: note, CreatedBy: actor.ID}
	if in.ExpiresOn != "" {
		d, err := domain.ParseDate(in.ExpiresOn)
		if err != nil {
			return nil, domain.Invalid("expiresOn", "Tanggal tidak valid.")
		}
		now := s.clock.Now()
		end := time.Date(d.Year(), d.Month(), d.Day()+1, 0, 0, 0, 0, now.Location())
		if !end.After(now) {
			return nil, domain.Invalid("expiresOn", "Batas pemakaian harus hari ini atau setelahnya.")
		}
		p.ExpiresAt = &end
	}
	if err := s.promos.Create(ctx, p); err != nil {
		var app *domain.AppError
		if errors.As(err, &app) && app.Code == "PROMO_EXISTS" {
			return nil, &domain.AppError{Kind: app.Kind, Code: app.Code, Message: app.Message, Field: "code"}
		}
		return nil, err
	}
	s.audit.Record(AuditEntry{UserID: &actor.ID, Email: actor.Email, Event: domain.EventPromoCreated, Client: client,
		Detail: "Kode " + p.Code + " · kuota " + itoa(p.MaxUses) + noteSuffix(note)})
	return p, nil
}

func (s *MembershipService) ListPromos(ctx context.Context) ([]domain.PromoCode, error) {
	return s.promos.List(ctx)
}

func (s *MembershipService) SetPromoActive(ctx context.Context, id int64, active bool) error {
	ok, err := s.promos.SetActive(ctx, id, active)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NotFound("Kode tidak ditemukan.")
	}
	return nil
}
