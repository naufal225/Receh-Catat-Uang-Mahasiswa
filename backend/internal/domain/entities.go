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
