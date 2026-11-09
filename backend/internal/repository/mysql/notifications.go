package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"receh/internal/domain"
)

type NotificationRepo struct{ db *sql.DB }

func NewNotificationRepo(db *sql.DB) *NotificationRepo { return &NotificationRepo{db} }

func (r *NotificationRepo) Insert(ctx context.Context, n *domain.Notification) (bool, error) {
	n.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (user_id, kind, title, body, link, ref_key, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		n.UserID, n.Kind, n.Title, n.Body, n.Link, n.RefKey, unix(n.CreatedAt))
	if isUnique(err) {
		return false, nil // Already notified about this event.
	}
	if err != nil {
		return false, err
	}
	n.ID, err = res.LastInsertId()
	return true, err
}

func (r *NotificationRepo) List(ctx context.Context, userID int64, limit int) ([]domain.Notification, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, kind, title, body, link, ref_key, read_at, created_at FROM notifications
		 WHERE user_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Notification{}
	for rows.Next() {
		var n domain.Notification
		var read sql.NullInt64
		var created int64
		if err := rows.Scan(&n.ID, &n.UserID, &n.Kind, &n.Title, &n.Body, &n.Link, &n.RefKey, &read, &created); err != nil {
			return nil, err
		}
		if read.Valid {
			t := fromUnix(read.Int64)
			n.ReadAt = &t
		}
		n.CreatedAt = fromUnix(created)
		out = append(out, n)
	}
	return out, rows.Err()
}

func (r *NotificationRepo) UnreadCount(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE user_id = ? AND read_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (r *NotificationRepo) MarkRead(ctx context.Context, userID int64, ids []int64, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	args := []any{at.Unix(), userID}
	for _, id := range ids {
		args = append(args, id)
	}
	_, err := r.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL AND id IN (?`+
		strings.Repeat(",?", len(ids)-1)+`)`, args...)
	return err
}

func (r *NotificationRepo) MarkAllRead(ctx context.Context, userID int64, at time.Time) error {
	_, err := r.db.ExecContext(ctx, `UPDATE notifications SET read_at = ? WHERE user_id = ? AND read_at IS NULL`, at.Unix(), userID)
	return err
}

func (r *NotificationRepo) DeleteBefore(ctx context.Context, t time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM notifications WHERE created_at < ?`, t.Unix())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *NotificationRepo) Prefs(ctx context.Context, userID int64) (domain.NotificationPrefs, error) {
	p := domain.NotificationPrefs{Disabled: map[domain.NotificationKind]bool{},
		LowBalanceThreshold: domain.DefaultLowBalance, ReminderHour: domain.DefaultReminderHour}
	var disabled string
	err := r.db.QueryRowContext(ctx,
		`SELECT disabled_kinds, low_balance_threshold, reminder_hour FROM notification_prefs WHERE user_id = ?`, userID).
		Scan(&disabled, &p.LowBalanceThreshold, &p.ReminderHour)
	if err == sql.ErrNoRows {
		return p, nil // Defaults: everything on.
	}
	if err != nil {
		return p, err
	}
	for _, k := range strings.Split(disabled, ",") {
		if k != "" {
			p.Disabled[domain.NotificationKind(k)] = true
		}
	}
	return p, nil
}

func (r *NotificationRepo) SavePrefs(ctx context.Context, userID int64, p domain.NotificationPrefs) error {
	var off []string
	for _, k := range domain.AllNotificationKinds {
		if p.Disabled[k] {
			off = append(off, string(k))
		}
	}
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO notification_prefs (user_id, disabled_kinds, low_balance_threshold, reminder_hour) VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE disabled_kinds = VALUES(disabled_kinds),
		    low_balance_threshold = VALUES(low_balance_threshold), reminder_hour = VALUES(reminder_hour)`,
		userID, strings.Join(off, ","), p.LowBalanceThreshold, p.ReminderHour)
	return err
}
