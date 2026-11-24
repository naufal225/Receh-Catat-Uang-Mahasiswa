package domain

import (
	"context"
	"time"
)

// ---- Catalog --------------------------------------------------------------------

// Interval is how long one payment buys.
type Interval string

const (
	IntervalMonth Interval = "month"
	IntervalYear  Interval = "year"
)

// PricePoint is one thing a workspace owner can buy: a plan for one interval.
type PricePoint struct {
	ID       string            `json:"id"`
	Plan     Plan              `json:"plan"`
	Interval Interval          `json:"interval"`
	Amount   int64             `json:"amount"` // Whole units of Catalog.Currency (rupiah have no cents).
	Label    map[string]string `json:"label"`
}

// Extend returns when a period of this price that starts at `from` ends.
func (p PricePoint) Extend(from time.Time) time.Time {
	if p.Interval == IntervalYear {
		return from.AddDate(1, 0, 0)
	}
	return from.AddDate(0, 1, 0)
}

// Catalog is what the product sells. It is plain data (see config/catalog.go),
// so a buyer of the template changes prices without touching code.
type Catalog struct {
	// PlanName is how the paid plan is called on screen, by language.
	PlanName map[string]string `json:"planName"`
	Currency string            `json:"currency"`
	Prices   []PricePoint      `json:"prices"`
	// GraceDays of access stay after a period ends unpaid, before the workspace
	// goes back to the free plan.
	GraceDays int `json:"graceDays"`
	// RenewalNoticeDays before a period ends the owner is reminded to renew.
	RenewalNoticeDays int `json:"renewalNoticeDays"`
	// InvoiceTTL is how long a checkout link stays payable.
	InvoiceTTL time.Duration `json:"-"`
}

func (c Catalog) Price(id string) (PricePoint, bool) {
	for _, p := range c.Prices {
		if p.ID == id {
			return p, true
		}
	}
	return PricePoint{}, false
}

func (c Catalog) Grace() time.Duration { return time.Duration(c.GraceDays) * 24 * time.Hour }

// ---- Payments and subscriptions --------------------------------------------------

type PaymentStatus string

const (
	PaymentPending PaymentStatus = "pending"
	PaymentPaid    PaymentStatus = "paid"
	PaymentExpired PaymentStatus = "expired"
	PaymentFailed  PaymentStatus = "failed"
)

// Payment is one checkout: a request to pay for one period of one workspace.
type Payment struct {
	ID          int64         `json:"id"`
	OrgID       int64         `json:"orgId"`
	OrgName     string        `json:"orgName"`
	PayerID     *int64        `json:"-"`
	PayerEmail  string        `json:"payerEmail"`
	PriceID     string        `json:"priceId"`
	Amount      int64         `json:"amount"`
	Currency    string        `json:"currency"`
	Provider    string        `json:"provider"`
	ProviderRef string        `json:"-"`
	ExternalID  string        `json:"externalId"`
	Status      PaymentStatus `json:"status"`
	CheckoutURL string        `json:"checkoutUrl,omitempty"`
	PeriodStart *time.Time    `json:"periodStart"`
	PeriodEnd   *time.Time    `json:"periodEnd"`
	ExpiresAt   time.Time     `json:"expiresAt"`
	PaidAt      *time.Time    `json:"paidAt"`
	CreatedAt   time.Time     `json:"createdAt"`
}

// SubState is where a subscription stands at a given moment.
type SubState string

const (
	SubActive  SubState = "active"  // Paid period running, will renew when the owner pays again.
	SubEnding  SubState = "ending"  // Paid period running, owner chose not to renew.
	SubGrace   SubState = "grace"   // Period over, access kept for a few days.
	SubExpired SubState = "expired" // Back on the free plan.
	SubNone    SubState = "none"    // Never subscribed.
)

// Subscription is the paid state of one workspace. Paying again extends it.
type Subscription struct {
	OrgID             int64     `json:"orgId"`
	PriceID           string    `json:"priceId"`
	PeriodEnd         time.Time `json:"periodEnd"`
	CancelAtPeriodEnd bool      `json:"cancelAtPeriodEnd"`
	LastPaymentID     *int64    `json:"-"`
	// Stored is the state the sweeper last wrote; State() is always current.
	Stored    SubState  `json:"-"`
	UpdatedAt time.Time `json:"-"`
}

// State derives the current state from the clock, so it is right even when
// the background sweep has not run yet.
func (s *Subscription) State(now time.Time, grace time.Duration) SubState {
	switch {
	case s == nil:
		return SubNone
	case now.Before(s.PeriodEnd) && s.CancelAtPeriodEnd:
		return SubEnding
	case now.Before(s.PeriodEnd):
		return SubActive
	case now.Before(s.PeriodEnd.Add(grace)):
		return SubGrace
	default:
		return SubExpired
	}
}

// ---- Payment provider port --------------------------------------------------------

// CheckoutRequest asks a gateway for a page where the owner can pay.
type CheckoutRequest struct {
	ExternalID  string // Our unique id; the gateway echoes it back.
	Amount      int64
	Currency    string
	Description string
	PayerEmail  string
	PayerName   string
	ExpiresAt   time.Time
	// ReturnURL is where the gateway sends the browser after paying.
	ReturnURL string
}

// Checkout is what the gateway returns: send the browser to URL.
type Checkout struct {
	ProviderRef string
	URL         string
}

// ProviderPayment is a gateway's own view of one payment.
type ProviderPayment struct {
	Status PaymentStatus
	Amount int64
	PaidAt *time.Time
}

// WebhookEvent is a verified notification from a gateway. It is only a hint:
// the service asks the gateway again (Fetch) before it grants anything.
type WebhookEvent struct {
	// EventID is unique per delivery attempt of one event, so replays are ignored.
	EventID    string
	ExternalID string
}

// PaymentProvider is one payment gateway. Implementations live in
// platform/payment; nothing else knows a gateway's API.
type PaymentProvider interface {
	Name() string
	CreateCheckout(ctx context.Context, req CheckoutRequest) (Checkout, error)
	// Cancel stops a checkout from being paid (best effort).
	Cancel(ctx context.Context, externalID, providerRef string) error
	// Fetch asks the gateway what happened to a payment.
	Fetch(ctx context.Context, externalID, providerRef string) (ProviderPayment, error)
	// ParseWebhook verifies the signature and extracts which payment changed.
	// header returns a request header by name.
	ParseWebhook(raw []byte, header func(string) string) (WebhookEvent, error)
}

// ---- Repository port ---------------------------------------------------------------

// SettleInput describes what the gateway says about one payment.
type SettleInput struct {
	PaymentID int64
	// Provider and EventID make the call idempotent: the same pair is applied
	// once. EventID empty means "no event" (a sync, which is idempotent by state).
	Provider string
	EventID  string
	Status   PaymentStatus
	PaidAt   time.Time
	Price    PricePoint
	Grace    time.Duration
	Now      time.Time
	// Note labels the entitlement an accepted payment creates.
	Note string
}

// SettleResult tells what Settle did.
type SettleResult struct {
	Payment *Payment
	// Duplicate: the event was seen before; nothing changed.
	Duplicate bool
	// Activated: this call turned the payment into a paid period.
	Activated    bool
	Subscription *Subscription
}

type BillingRepository interface {
	// Reserve inserts p as the workspace's one pending checkout unless one that
	// is still payable exists; then it returns that one and created=false.
	Reserve(ctx context.Context, p *Payment, now time.Time) (cur *Payment, created bool, err error)
	SetCheckout(ctx context.Context, id int64, providerRef, url string) error
	// Fail marks a pending payment failed (the gateway refused to create it).
	Fail(ctx context.Context, id int64) error
	// Settle applies the gateway's word on a payment in one transaction.
	Settle(ctx context.Context, in SettleInput) (*SettleResult, error)

	PaymentByID(ctx context.Context, id int64) (*Payment, error)
	PaymentByExternalID(ctx context.Context, externalID string) (*Payment, error)
	PendingFor(ctx context.Context, orgID int64, now time.Time) (*Payment, error)
	PaymentsForOrg(ctx context.Context, orgID int64, limit int) ([]Payment, error)
	PaymentsAll(ctx context.Context, status PaymentStatus, limit, offset int) ([]Payment, int, error)
	// PendingBefore lists pending payments created before t (oldest first).
	PendingBefore(ctx context.Context, t time.Time, limit int) ([]Payment, error)

	SubscriptionOf(ctx context.Context, orgID int64) (*Subscription, error)
	// OwnedSubscriptions lists the subscriptions of workspaces a user owns.
	OwnedSubscriptions(ctx context.Context, userID int64) ([]OwnedSubscription, error)
	SetCancelAtPeriodEnd(ctx context.Context, orgID int64, cancel bool) (bool, error)
	// MarkStale stores the derived state of subscriptions whose period is over.
	SyncSubscriptionStates(ctx context.Context, now time.Time, grace time.Duration) error
}

// OwnedSubscription is a subscription together with its workspace's name.
type OwnedSubscription struct {
	Subscription
	OrgName  string
	Personal bool
}

// Audit events for billing.
const (
	EventPaymentCreated      AuditEvent = "payment_created"
	EventPaymentPaid         AuditEvent = "payment_paid"
	EventPaymentExpired      AuditEvent = "payment_expired"
	EventPaymentMismatch     AuditEvent = "payment_mismatch"
	EventSubscriptionStopped AuditEvent = "subscription_stopped"
	EventSubscriptionResumed AuditEvent = "subscription_resumed"
)

// ErrBadSignature is returned by a provider when a webhook is not authentic.
var ErrBadSignature = newErr(KindUnauthorized, "BAD_SIGNATURE", "Tanda tangan webhook tidak valid.")

// BillingOff explains a refusal when the edition has no payments.
func BillingOff() *AppError {
	return Invalid("", "Edisi ini membuka semua fitur; pembayaran tidak diperlukan.")
}
