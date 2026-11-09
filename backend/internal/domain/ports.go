package domain

import (
	"context"
	"time"
)

// Repository interfaces are the ports the use cases need. The mysql package
// implements them, and repository/cached wraps some of them with caches.

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	CreateWithDefaults(ctx context.Context, u *User, defaults []Category) error
	ByID(ctx context.Context, id int64) (*User, error)
	ByEmail(ctx context.Context, email string) (*User, error)
	UpdatePassword(ctx context.Context, id int64, hash string) error
	UpdateStatus(ctx context.Context, id int64, status UserStatus, reason string) error
	List(ctx context.Context, f UserFilter) (Page[User], error)
	CountAdmins(ctx context.Context) (int, error)
	ByGoogleSub(ctx context.Context, sub string) (*User, error)
	// ActiveUserIDs lists active (not suspended or frozen) user accounts, for
	// background jobs that run per user.
	ActiveUserIDs(ctx context.Context) ([]int64, error)
	// LinkGoogle attaches a Google account to a user that has none yet.
	LinkGoogle(ctx context.Context, id int64, sub string) error
}

// OAuthProvider is an OpenID Connect identity provider (Google). The flow is
// authorization code with PKCE: AuthURL sends the browser away with a code
// challenge, Exchange trades the returned code plus the verifier for who the
// person is, over a direct server-to-provider call.
type OAuthProvider interface {
	AuthURL(state, codeChallenge string) string
	Exchange(ctx context.Context, code, codeVerifier string) (*OAuthIdentity, error)
}

type SessionRepository interface {
	Create(ctx context.Context, s *Session) error
	ByTokenHash(ctx context.Context, hash string) (*Session, error)
	ListActive(ctx context.Context, userID int64) ([]Session, error)
	Revoke(ctx context.Context, userID, sessionID int64) (bool, error)
	RevokeOthers(ctx context.Context, userID, keepID int64) (int64, error)
	RevokeAll(ctx context.Context, userID int64) error
	Touch(ctx context.Context, lastSeen map[int64]time.Time) error
	SetLocation(ctx context.Context, sessionID int64, location string) error
	DeleteExpired(ctx context.Context) error
}

type WalletRepository interface {
	Create(ctx context.Context, w *Wallet) error
	List(ctx context.Context, userID int64, includeArchived bool) ([]Wallet, error)
	ByID(ctx context.Context, userID, id int64) (*Wallet, error)
	Update(ctx context.Context, w *Wallet) error
	HasTransactions(ctx context.Context, id int64) (bool, error)
	Delete(ctx context.Context, id int64) error
	Archive(ctx context.Context, id int64) error
}

type CategoryRepository interface {
	Create(ctx context.Context, c *Category) error
	List(ctx context.Context, userID int64) ([]Category, error)
	ByID(ctx context.Context, userID, id int64) (*Category, error)
	ByName(ctx context.Context, userID int64, t TxType, name string) (*Category, error)
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
	ByID(ctx context.Context, userID, id int64) (*Transaction, error)
	List(ctx context.Context, userID int64, f TxFilter) (Page[Transaction], error)
	Latest(ctx context.Context, userID, walletID int64, limit int) ([]Transaction, error)
	AllBetween(ctx context.Context, userID int64, from, to string) ([]Transaction, error)
	SumByDay(ctx context.Context, userID, walletID int64, from, to string) ([]DaySum, error)
	SumByCategory(ctx context.Context, userID, walletID int64, t TxType, from, to string) ([]CategorySlice, error)
	WalletSummaries(ctx context.Context, userID int64, from, to string) ([]WalletSummary, error)
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
	ByID(ctx context.Context, userID, id int64) (*Recurring, error)
	List(ctx context.Context, userID int64) ([]Recurring, error)
	Delete(ctx context.Context, userID, id int64) (bool, error)
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
	ByID(ctx context.Context, userID, id int64) (*Transfer, error)
	// LastFee is the fee of the most recent transfer on this route, so a fee
	// that depends on the bank pair is remembered without a settings table.
	LastFee(ctx context.Context, userID, fromWalletID, toWalletID int64) (fee int64, found bool, err error)
}

// ChangeNotifier is told whenever a user's financial data changes, so caches
// of derived views (the dashboard) know to stop serving the old answer.
type ChangeNotifier interface {
	Changed(userID int64)
}

type AuditRepository interface {
	Insert(ctx context.Context, l *AuditLog) error
	SetLocation(ctx context.Context, id int64, location string) error
	List(ctx context.Context, f AuditFilter) (Page[AuditLog], error)
}

// GeoLocator turns an IP address into a human readable place.
type GeoLocator interface {
	Locate(ctx context.Context, ip string) string
}

// Clock lets tests control "today".
type Clock interface {
	Now() time.Time
	Today() string
}
