package domain

import (
	"context"
	"time"
)

// Repository interfaces are the ports the use cases need. The mysql package
// implements them, and repository/cached wraps some of them with caches.

type UserRepository interface {
	Create(ctx context.Context, u *User) error
	// Delete removes a user; used to undo a sign-up whose module setup failed.
	Delete(ctx context.Context, id int64) error
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

// ChangeNotifier is told whenever a workspace's data changes, so caches
// of derived views (the dashboard) know to stop serving the old answer.
type ChangeNotifier interface {
	Changed(orgID int64)
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
