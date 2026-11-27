package model

import (
	"context"
	"time"

	"receh/internal/domain"
)

// Finance module types. Core types (users, sessions, audit) live in package
// domain; everything about money lives here, so the module can be removed.

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
	OrgID    int64      `json:"-"`
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
	ID    int64  `json:"id"`
	OrgID int64  `json:"-"`
	Name  string `json:"name"`
	Type  TxType `json:"type"`
	Color string `json:"color"`
}

// Transaction is one income or expense. OccurredOn is a calendar date
// (YYYY-MM-DD) because that is how people think about spending, and it keeps
// monthly grouping free of timezone arithmetic.
type Transaction struct {
	ID            int64  `json:"id"`
	OrgID         int64  `json:"-"`
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
	OrgID          int64     `json:"-"`
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
	OrgID         int64     `json:"-"`
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

// ---- Repository ports --------------------------------------------------------

type WalletRepository interface {
	Create(ctx context.Context, w *Wallet) error
	List(ctx context.Context, orgID int64, includeArchived bool) ([]Wallet, error)
	ByID(ctx context.Context, orgID, id int64) (*Wallet, error)
	Update(ctx context.Context, w *Wallet) error
	HasTransactions(ctx context.Context, id int64) (bool, error)
	Delete(ctx context.Context, id int64) error
	Archive(ctx context.Context, id int64) error
}

type CategoryRepository interface {
	Create(ctx context.Context, c *Category) error
	List(ctx context.Context, orgID int64) ([]Category, error)
	ByID(ctx context.Context, orgID, id int64) (*Category, error)
	ByName(ctx context.Context, orgID int64, t TxType, name string) (*Category, error)
	Update(ctx context.Context, c *Category) error
	InUse(ctx context.Context, id int64) (bool, error)
	Delete(ctx context.Context, id int64) error
}

// TransactionRepository applies balance changes atomically: the row change and
// the wallet update commit together, or neither does. A balance that would go
// below zero returns ErrInsufficient.
type TransactionRepository interface {
	Create(ctx context.Context, t *Transaction) error
	Update(ctx context.Context, old, updated *Transaction) error
	Delete(ctx context.Context, t *Transaction) error
	ByID(ctx context.Context, orgID, id int64) (*Transaction, error)
	List(ctx context.Context, orgID int64, f TxFilter) (domain.Page[Transaction], error)
	Latest(ctx context.Context, orgID, walletID int64, limit int) ([]Transaction, error)
	AllBetween(ctx context.Context, orgID int64, from, to string) ([]Transaction, error)
	SumByDay(ctx context.Context, orgID, walletID int64, from, to string) ([]DaySum, error)
	SumByCategory(ctx context.Context, orgID, walletID int64, t TxType, from, to string) ([]CategorySlice, error)
	WalletSummaries(ctx context.Context, orgID int64, from, to string) ([]WalletSummary, error)
}

// DaySum is the income and expense total of one calendar date.
type DaySum struct {
	Day     string
	Income  int64
	Expense int64
}

type RecurringRepository interface {
	Create(ctx context.Context, r *Recurring) error
	Update(ctx context.Context, r *Recurring) error
	ByID(ctx context.Context, orgID, id int64) (*Recurring, error)
	List(ctx context.Context, orgID int64) ([]Recurring, error)
	Delete(ctx context.Context, orgID, id int64) (bool, error)
	// Due lists active rules of active users whose NextDue is on or before day.
	Due(ctx context.Context, day string) ([]Recurring, error)
	// PostOccurrence inserts the transaction and advances NextDue in one commit.
	// The (recurring_id, occurred_on) unique index makes a repeat a harmless no-op.
	PostOccurrence(ctx context.Context, r *Recurring, on string, nextDue string) error
	SetLastError(ctx context.Context, id int64, msg string) error
	DeactivateForWallet(ctx context.Context, walletID int64) error
}

// TransferRepository writes a transfer, its two legs and its fee expense in one
// commit, so balances never show money that left one wallet but did not reach
// the other. A source that cannot cover amount + fee returns ErrInsufficient.
type TransferRepository interface {
	Create(ctx context.Context, t *Transfer) error
	Update(ctx context.Context, old, updated *Transfer) error
	Delete(ctx context.Context, t *Transfer) error
	ByID(ctx context.Context, orgID, id int64) (*Transfer, error)
	// LastFee is the fee of the most recent transfer on this route, so a fee
	// that depends on the bank pair is remembered without a settings table.
	LastFee(ctx context.Context, orgID, fromWalletID, toWalletID int64) (fee int64, found bool, err error)
}

// ChangeNotifier is told whenever a user's financial data changes, so caches

// ErrInsufficient: a balance change would take a wallet below zero.
var ErrInsufficient = domain.Conflict("INSUFFICIENT_BALANCE", "Saldo dompet tidak cukup untuk transaksi ini.")

// NextOccurrence computes the date after `from` for a schedule. Monthly and
// yearly rules clamp to the end of short months: a rule anchored on the 31st
// runs on 28 Feb, then returns to 31 Mar.
func NextOccurrence(from string, f Frequency, anchorDay int) (string, error) {
	d, err := domain.ParseDate(from)
	if err != nil {
		return "", err
	}
	var next time.Time
	switch f {
	case FreqDaily:
		next = d.AddDate(0, 0, 1)
	case FreqWeekly:
		next = d.AddDate(0, 0, 7)
	case FreqMonthly:
		next = clampedDate(d.Year(), d.Month()+1, anchorDay)
	case FreqYearly:
		next = clampedDate(d.Year()+1, d.Month(), anchorDay)
	default:
		next = clampedDate(d.Year(), d.Month()+1, anchorDay)
	}
	return next.Format(domain.DateLayout), nil
}

func clampedDate(year int, month time.Month, day int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return first.AddDate(0, 0, day-1)
}
