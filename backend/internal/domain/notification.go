package domain

import (
	"context"
	"slices"
	"time"
)

// NotificationKind names one kind of in-app reminder. Each can be switched off.
type NotificationKind string

const (
	NotifyBillDue       NotificationKind = "bill_due"       // a recurring bill is due tomorrow
	NotifyBillFailed    NotificationKind = "bill_failed"    // a bill could not post: balance too low
	NotifyBudgetWarning NotificationKind = "budget_warning" // a budget passed 80%
	NotifyBudgetOver    NotificationKind = "budget_over"    // a budget passed 100%
	NotifyLowBalance    NotificationKind = "low_balance"    // a wallet fell below the threshold
	NotifyGoalAchieved  NotificationKind = "goal_achieved"  // a savings goal reached its target
	NotifyDailyReminder NotificationKind = "daily_reminder" // nothing recorded today (evening)
	NotifyWeeklySummary NotificationKind = "weekly_summary" // last week in numbers (Monday)
)

// AllNotificationKinds is the order the settings page lists them in.
var AllNotificationKinds = []NotificationKind{
	NotifyBudgetWarning, NotifyBudgetOver, NotifyBillDue, NotifyBillFailed,
	NotifyLowBalance, NotifyGoalAchieved, NotifyDailyReminder, NotifyWeeklySummary,
}

func (k NotificationKind) Valid() bool { return slices.Contains(AllNotificationKinds, k) }

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

// NotificationPrefs are a user's choices. Kinds missing from Disabled are on.
type NotificationPrefs struct {
	Disabled            map[NotificationKind]bool `json:"-"`
	LowBalanceThreshold int64                     `json:"lowBalanceThreshold"`
	// ReminderHour is the local hour after which the daily reminder may fire.
	ReminderHour int `json:"reminderHour"`
}

const (
	DefaultLowBalance   int64 = 50_000
	DefaultReminderHour int   = 20
)

func (p NotificationPrefs) Enabled(k NotificationKind) bool { return !p.Disabled[k] }

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
