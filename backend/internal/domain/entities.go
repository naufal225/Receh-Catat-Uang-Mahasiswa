// Package domain holds the business entities, rules and the ports (interfaces)
// the rest of the application depends on. It imports nothing from the outer
// layers: no SQL, no HTTP.
package domain

import "time"

// Role separates people who manage money from people who manage the platform.
type Role string

const (
	RoleUser  Role = "user"
	RoleAdmin Role = "admin"
)

// UserStatus is the moderation state of an account.
//
//	active    full access
//	suspended cannot log in at all, every session is revoked (ditangguhkan)
//	frozen    can log in and read or export data, but nothing can change (dibekukan)
type UserStatus string

const (
	StatusActive    UserStatus = "active"
	StatusSuspended UserStatus = "suspended"
	StatusFrozen    UserStatus = "frozen"
)

func (s UserStatus) Valid() bool {
	return s == StatusActive || s == StatusSuspended || s == StatusFrozen
}

type User struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	PasswordHash string `json:"-"`
	// GoogleSub is Google's stable account id once the user signed in with Google.
	GoogleSub    string     `json:"-"`
	Role         Role       `json:"role"`
	Status       UserStatus `json:"status"`
	StatusReason string     `json:"statusReason"`
	CreatedAt    time.Time  `json:"createdAt"`
	// Derived for the client: which sign-in methods this account has.
	HasPassword  bool `json:"hasPassword"`
	GoogleLinked bool `json:"googleLinked"`
	// Plan is filled only where it is shown (the admin user list).
	Plan Plan `json:"plan,omitempty"`
}

// OAuthIdentity is who an identity provider says the person is.
type OAuthIdentity struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
}

// Session is one logged-in device. Only the hash of the token is ever stored.
type Session struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"-"`
	TokenHash  string    `json:"-"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"-"`
	Device     string    `json:"device"`
	Location   string    `json:"location"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Current    bool      `json:"current"`
}

type WalletType string

const (
	WalletBank    WalletType = "bank"
	WalletEWallet WalletType = "ewallet"
	WalletCash    WalletType = "cash"
	WalletOther   WalletType = "other"
)

func (t WalletType) Valid() bool {
	return t == WalletBank || t == WalletEWallet || t == WalletCash || t == WalletOther
}

// Wallet is a place money lives: a bank account, an e-wallet or cash.
// Amounts are whole rupiah so no floating point ever touches a balance.
type Wallet struct {
	ID       int64      `json:"id"`
	UserID   int64      `json:"-"`
	Name     string     `json:"name"`
	Type     WalletType `json:"type"`
	Color    string     `json:"color"`
	Balance  int64      `json:"balance"`
	Archived bool       `json:"archived"`
}

type TxType string

const (
	TxIncome  TxType = "income"
	TxExpense TxType = "expense"
	// The two legs of a transfer. They move money between a user's own wallets,
	// so they never count as income or expense in charts and totals.
	TxTransferOut TxType = "transfer_out"
	TxTransferIn  TxType = "transfer_in"
	// TxTransfer is only a list filter matching both legs.
	TxTransfer TxType = "transfer"
)

// Valid reports whether a person may record this type directly. Transfer legs
// are written only by the transfer use case.
func (t TxType) Valid() bool { return t == TxIncome || t == TxExpense }

func (t TxType) IsTransfer() bool { return t == TxTransferOut || t == TxTransferIn }

// Delta is the signed effect of an amount of this type on a wallet balance.
func (t TxType) Delta(amount int64) int64 {
	if t == TxIncome || t == TxTransferIn {
		return amount
	}
	return -amount
}

type Category struct {
	ID     int64  `json:"id"`
	UserID int64  `json:"-"`
	Name   string `json:"name"`
	Type   TxType `json:"type"`
	Color  string `json:"color"`
}

// Transaction is one income or expense. OccurredOn is a calendar date
// (YYYY-MM-DD) because that is how people think about spending, and it keeps
// monthly grouping free of timezone arithmetic.
type Transaction struct {
	ID            int64  `json:"id"`
	UserID        int64  `json:"-"`
	WalletID      int64  `json:"walletId"`
	WalletName    string `json:"walletName"`
	WalletColor   string `json:"walletColor"`
	CategoryID    int64  `json:"categoryId"`
	CategoryName  string `json:"categoryName"`
	CategoryColor string `json:"categoryColor"`
	Type          TxType `json:"type"`
	Name          string `json:"name"`
	Amount        int64  `json:"amount"`
	OccurredOn    string `json:"occurredOn"`
	RecurringID   *int64 `json:"recurringId"`
	// TransferID is set on both legs of a transfer and on its fee expense. Such
	// rows change only through the transfer they belong to.
	TransferID *int64 `json:"transferId"`
	// ImportBatchID is set on transactions created by a statement import.
	ImportBatchID *int64 `json:"importBatchId,omitempty"`
	// For a transfer leg: the wallet on the other side, and the fee the source paid.
	CounterpartName  string    `json:"counterpartName,omitempty"`
	CounterpartColor string    `json:"counterpartColor,omitempty"`
	TransferFee      int64     `json:"transferFee,omitempty"`
	CreatedAt        time.Time `json:"createdAt"`
}

// Transfer moves Amount from one wallet to another. Fee is charged to the
// source on top of Amount (top up GoPay Rp100.000 with a Rp1.000 fee takes
// Rp101.000 from BCA) and is recorded as an expense in the fee category.
type Transfer struct {
	ID             int64     `json:"id"`
	UserID         int64     `json:"-"`
	FromWalletID   int64     `json:"fromWalletId"`
	FromWalletName string    `json:"fromWalletName"`
	ToWalletID     int64     `json:"toWalletId"`
	ToWalletName   string    `json:"toWalletName"`
	Amount         int64     `json:"amount"`
	Fee            int64     `json:"fee"`
	Note           string    `json:"note"`
	OccurredOn     string    `json:"occurredOn"`
	CreatedAt      time.Time `json:"createdAt"`
	// FeeCategoryID is where the fee expense is booked. Not exposed.
	FeeCategoryID int64 `json:"-"`
}

// Total is what leaves the source wallet.
func (t *Transfer) Total() int64 { return t.Amount + t.Fee }

type Frequency string

const (
	FreqDaily   Frequency = "daily"
	FreqWeekly  Frequency = "weekly"
	FreqMonthly Frequency = "monthly"
	FreqYearly  Frequency = "yearly"
)

func (f Frequency) Valid() bool {
	return f == FreqDaily || f == FreqWeekly || f == FreqMonthly || f == FreqYearly
}

// PerMonth is the monthly equivalent of an amount at this frequency, used to
// show how much a student has committed each month.
func (f Frequency) PerMonth(amount int64) int64 {
	switch f {
	case FreqDaily:
		return amount * 30
	case FreqWeekly:
		return amount * 52 / 12
	case FreqYearly:
		return amount / 12
	}
	return amount
}

// Recurring is a planned income or expense posted automatically on schedule.
// AnchorDay remembers the intended day of month so a rule on the 31st returns
// to the 31st after passing through a shorter month.
type Recurring struct {
	ID            int64     `json:"id"`
	UserID        int64     `json:"-"`
	WalletID      int64     `json:"walletId"`
	WalletName    string    `json:"walletName"`
	WalletColor   string    `json:"walletColor"`
	CategoryID    int64     `json:"categoryId"`
	CategoryName  string    `json:"categoryName"`
	CategoryColor string    `json:"categoryColor"`
	Type          TxType    `json:"type"`
	Name          string    `json:"name"`
	Amount        int64     `json:"amount"`
	Frequency     Frequency `json:"frequency"`
	AnchorDay     int       `json:"anchorDay"`
	NextDue       string    `json:"nextDue"`
	Active        bool      `json:"active"`
	LastError     string    `json:"lastError"`
}

// AuditEvent names what happened in an audit log row.
type AuditEvent string

const (
	EventRegister        AuditEvent = "register"
	EventLoginSuccess    AuditEvent = "login_success"
	EventLoginFailed     AuditEvent = "login_failed"
	EventLoginBlocked    AuditEvent = "login_blocked"
	EventLogout          AuditEvent = "logout"
	EventSessionRevoked  AuditEvent = "session_revoked"
	EventPasswordChanged AuditEvent = "password_changed"
	EventGoogleLinked    AuditEvent = "google_linked"
	EventUserCreated     AuditEvent = "admin_user_created"
	EventStatusChanged   AuditEvent = "admin_status_changed"
)

type AuditLog struct {
	ID        int64      `json:"id"`
	UserID    *int64     `json:"userId"`
	Email     string     `json:"email"`
	Event     AuditEvent `json:"event"`
	IP        string     `json:"ip"`
	Device    string     `json:"device"`
	Location  string     `json:"location"`
	Detail    string     `json:"detail"`
	CreatedAt time.Time  `json:"createdAt"`
}

// ClientInfo describes the device a request came from.
type ClientInfo struct {
	IP        string
	UserAgent string
	Device    string
}

// ---- Read models -----------------------------------------------------------

type Page[T any] struct {
	Items    []T `json:"items"`
	Total    int `json:"total"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

type TxFilter struct {
	Search     string
	Type       TxType
	WalletID   int64
	CategoryID int64
	From       string
	To         string
	Page       int
	PageSize   int
}

type AuditFilter struct {
	Search   string
	Event    string
	Page     int
	PageSize int
}

type UserFilter struct {
	Search   string
	Status   string
	Page     int
	PageSize int
}

// Bucket is one bar group on the dashboard.
type Bucket struct {
	Key     string `json:"key"`
	Label   string `json:"label"`
	From    string `json:"from"`
	To      string `json:"to"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
}

type CategorySlice struct {
	CategoryID int64  `json:"categoryId"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	Total      int64  `json:"total"`
}

type WalletSummary struct {
	Name    string `json:"name"`
	Income  int64  `json:"income"`
	Expense int64  `json:"expense"`
	// Money moved in from or out to the user's other wallets.
	TransferIn  int64 `json:"transferIn"`
	TransferOut int64 `json:"transferOut"`
	Balance     int64 `json:"balance"`
}

type Totals struct {
	Income  int64 `json:"income"`
	Expense int64 `json:"expense"`
}

type Dashboard struct {
	Unit             string          `json:"unit"`
	From             string          `json:"from"`
	To               string          `json:"to"`
	Title            string          `json:"title"`
	CanGoForward     bool            `json:"canGoForward"`
	Buckets          []Bucket        `json:"buckets"`
	Totals           Totals          `json:"totals"`
	ExpenseSlices    []CategorySlice `json:"expenseSlices"`
	IncomeSlices     []CategorySlice `json:"incomeSlices"`
	TotalBalance     int64           `json:"totalBalance"`
	Wallets          []Wallet        `json:"wallets"`
	Latest           []Transaction   `json:"latest"`
	UpcomingRoutines []Recurring     `json:"upcomingRoutines"`
	// Budgets of the current month (not affected by the wallet filter).
	Budgets []BudgetStatus `json:"budgets"`
	// Active savings goals.
	Goals []GoalProgress `json:"goals"`
}

type MonthlyReport struct {
	Month         string          `json:"month"`
	Title         string          `json:"title"`
	Owner         string          `json:"owner"`
	Totals        Totals          `json:"totals"`
	Wallets       []WalletSummary `json:"wallets"`
	ExpenseSlices []CategorySlice `json:"expenseSlices"`
	IncomeSlices  []CategorySlice `json:"incomeSlices"`
	Transactions  []Transaction   `json:"transactions"`
	GeneratedAt   time.Time       `json:"generatedAt"`
}
