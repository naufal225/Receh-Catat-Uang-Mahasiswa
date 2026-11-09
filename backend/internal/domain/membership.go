package domain

import (
	"context"
	"time"
)

// Plan is what a user may use. Everything a free user can do stays free; the
// premium plan lifts limits and unlocks extras.
type Plan string

const (
	PlanFree    Plan = "free"
	PlanPremium Plan = "premium"
)

// Unlimited marks a limit that does not apply.
const Unlimited = -1

// Limits are the numbers and switches one plan allows. Use cases check them;
// the UI only mirrors them so it can explain a refusal before it happens.
type Limits struct {
	MaxWallets int  `json:"maxWallets"`
	MaxBudgets int  `json:"maxBudgets"`
	MaxGoals   int  `json:"maxGoals"`
	CSVImport  bool `json:"csvImport"`
}

func LimitsFor(p Plan) Limits {
	if p == PlanPremium {
		return Limits{MaxWallets: Unlimited, MaxBudgets: Unlimited, MaxGoals: Unlimited, CSVImport: true}
	}
	return Limits{MaxWallets: 3, MaxBudgets: 3, MaxGoals: 1, CSVImport: false}
}

// Allows reports whether one more item fits under a limit.
func Allows(limit, current int) bool { return limit == Unlimited || current < limit }

// EntitlementSource says how a premium period was obtained.
type EntitlementSource string

const (
	SourcePaid EntitlementSource = "paid" // A payment (payment gateway, later).
	SourceComp EntitlementSource = "comp" // Granted by an admin, e.g. family.
	SourceCode EntitlementSource = "code" // Redeemed from a promo code.
)

// Entitlement is one premium period. EndsAt nil means it never ends; a revoked
// entitlement stays as history but no longer counts.
type Entitlement struct {
	ID          int64             `json:"id"`
	UserID      int64             `json:"userId"`
	Source      EntitlementSource `json:"source"`
	Plan        Plan              `json:"plan"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      *time.Time        `json:"endsAt"`
	Note        string            `json:"note"`
	GrantedBy   *int64            `json:"grantedBy"`
	PromoCode   string            `json:"promoCode,omitempty"`
	RevokedAt   *time.Time        `json:"revokedAt"`
	CreatedAt   time.Time         `json:"createdAt"`
	PromoCodeID *int64            `json:"-"`
}

// ActiveAt reports whether the entitlement grants premium at t.
func (e *Entitlement) ActiveAt(t time.Time) bool {
	return e.RevokedAt == nil && !t.Before(e.StartsAt) && (e.EndsAt == nil || t.Before(*e.EndsAt))
}

// Membership is what the app tells a user about their plan.
type Membership struct {
	Plan           Plan              `json:"plan"`
	BillingEnabled bool              `json:"billingEnabled"`
	Source         EntitlementSource `json:"source,omitempty"`
	EndsAt         *time.Time        `json:"endsAt"`
	Note           string            `json:"note,omitempty"`
	Limits         Limits            `json:"limits"`
	FreeLimits     Limits            `json:"freeLimits"`
	Usage          Usage             `json:"usage"`
}

// Usage counts what a limit applies to.
type Usage struct {
	Wallets int `json:"wallets"`
	Budgets int `json:"budgets"`
	Goals   int `json:"goals"`
}

// PromoCode lets the developer hand out premium, e.g. to family. DurationDays
// nil means the premium it grants never ends.
type PromoCode struct {
	ID           int64      `json:"id"`
	Code         string     `json:"code"`
	Plan         Plan       `json:"plan"`
	DurationDays *int       `json:"durationDays"`
	MaxUses      int        `json:"maxUses"`
	Used         int        `json:"used"`
	ExpiresAt    *time.Time `json:"expiresAt"`
	Note         string     `json:"note"`
	Active       bool       `json:"active"`
	CreatedBy    int64      `json:"-"`
	CreatedAt    time.Time  `json:"createdAt"`
}

type EntitlementRepository interface {
	Create(ctx context.Context, e *Entitlement) error
	// ActiveFor returns the active entitlement that lasts longest at time t.
	ActiveFor(ctx context.Context, userID int64, t time.Time) (*Entitlement, error)
	ListForUser(ctx context.Context, userID int64) ([]Entitlement, error)
	ByID(ctx context.Context, id int64) (*Entitlement, error)
	Revoke(ctx context.Context, id int64, at time.Time) (bool, error)
	// PremiumUserIDs reports which of the given users are premium at t.
	PremiumUserIDs(ctx context.Context, ids []int64, t time.Time) (map[int64]bool, error)
}

type PromoRepository interface {
	Create(ctx context.Context, p *PromoCode) error
	List(ctx context.Context) ([]PromoCode, error)
	SetActive(ctx context.Context, id int64, active bool) (bool, error)
	// Redeem atomically checks the code (active, not expired, uses left, not
	// yet used by this user), counts the use and creates the entitlement.
	Redeem(ctx context.Context, code string, userID int64, now time.Time) (*Entitlement, error)
}

var (
	ErrPromoInvalid  = newErr(KindInvalid, "PROMO_INVALID", "Kode tidak ditemukan atau sudah tidak berlaku.")
	ErrPromoUsedUp   = newErr(KindConflict, "PROMO_USED_UP", "Kuota kode ini sudah habis.")
	ErrPromoRedeemed = newErr(KindConflict, "PROMO_ALREADY_REDEEMED", "Kamu sudah pernah memakai kode ini.")
)

// PlanLimit explains a refusal caused by the free plan.
func PlanLimit(msg string) *AppError { return newErr(KindForbidden, "PLAN_LIMIT", msg) }

// Audit events for membership.
const (
	EventPremiumGranted AuditEvent = "premium_granted"
	EventPremiumRevoked AuditEvent = "premium_revoked"
	EventPromoCreated   AuditEvent = "promo_created"
	EventPromoRedeemed  AuditEvent = "promo_redeemed"
)
