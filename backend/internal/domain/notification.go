package domain

import (
	"context"
	"time"
)

// NotificationKind names one kind of in-app reminder. Modules declare their
// own kinds (NotificationKindDef); each can be switched off by the user.
type NotificationKind string

// NotificationKindDef describes a kind for the settings page and the inbox.
type NotificationKindDef struct {
	Key   NotificationKind  `json:"key"`
	Label map[string]string `json:"label"` // by language: "id", "en"
	Icon  string            `json:"icon"`
	Tone  string            `json:"tone"` // ok | warn | danger | neutral
}

// NotificationSetting is a number a module lets the user tune (a threshold,
// an hour). Kind tells the UI how to show it: "amount", "hour" or "number".
type NotificationSetting struct {
	Key     string            `json:"key"`
	Label   map[string]string `json:"label"`
	Kind    string            `json:"kind"`
	Default int64             `json:"default"`
	Min     int64             `json:"min"`
	Max     int64             `json:"max"`
}

// Notification is one message in a user's inbox. RefKey identifies the event
// it is about (e.g. "budget_over:12:2026-01"); the database keeps it unique per
// user, so the same event never notifies twice, whatever restarts or retries.
type Notification struct {
	ID        int64            `json:"id"`
	UserID    int64            `json:"-"`
	Kind      NotificationKind `json:"kind"`
	Title     string           `json:"title"`
	Body      string           `json:"body"`
	Link      string           `json:"link"`
	RefKey    string           `json:"-"`
	ReadAt    *time.Time       `json:"readAt"`
	CreatedAt time.Time        `json:"createdAt"`
}

// NotificationPrefs are a user's choices. Kinds missing from Disabled are on;
// settings missing from Settings use their declared default.
type NotificationPrefs struct {
	Disabled map[NotificationKind]bool
	Settings map[string]int64
}

func (p NotificationPrefs) Enabled(k NotificationKind) bool { return !p.Disabled[k] }

// Setting returns a module setting, or def when the user never changed it.
func (p NotificationPrefs) Setting(key string, def int64) int64 {
	if v, ok := p.Settings[key]; ok {
		return v
	}
	return def
}

type NotificationRepository interface {
	// Insert stores n unless a notification with the same (user, RefKey) exists;
	// it reports whether a new one was created.
	Insert(ctx context.Context, n *Notification) (bool, error)
	List(ctx context.Context, userID int64, limit int) ([]Notification, error)
	UnreadCount(ctx context.Context, userID int64) (int, error)
	MarkRead(ctx context.Context, userID int64, ids []int64, at time.Time) error
	MarkAllRead(ctx context.Context, userID int64, at time.Time) error
	DeleteBefore(ctx context.Context, t time.Time) (int64, error)
	Prefs(ctx context.Context, userID int64) (NotificationPrefs, error)
	SavePrefs(ctx context.Context, userID int64, p NotificationPrefs) error
}
