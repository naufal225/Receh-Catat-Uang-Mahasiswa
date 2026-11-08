package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"receh/internal/domain"
)

type AuditRepo struct{ db *sql.DB }

func NewAuditRepo(db *sql.DB) *AuditRepo { return &AuditRepo{db} }

func (r *AuditRepo) Insert(ctx context.Context, l *domain.AuditLog) error {
	l.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO audit_logs (user_id, email, event, ip, device, location, detail, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		l.UserID, l.Email, l.Event, l.IP, l.Device, l.Location, l.Detail, unix(l.CreatedAt))
	if err != nil {
		return err
	}
	l.ID, err = res.LastInsertId()
	return err
}

func (r *AuditRepo) SetLocation(ctx context.Context, id int64, location string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE audit_logs SET location = ? WHERE id = ?`, location, id)
	return err
}

func (r *AuditRepo) List(ctx context.Context, f domain.AuditFilter) (domain.Page[domain.AuditLog], error) {
	limit, offset, page := pageBounds(f.Page, f.PageSize)
	var where []string
	var args []any
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + escapeLike(s) + "%"
		where = append(where, `(email LIKE ? OR ip LIKE ? OR device LIKE ? OR location LIKE ?)`)
		args = append(args, like, like, like, like)
	}
	if f.Event != "" {
		where = append(where, `event = ?`)
		args = append(args, f.Event)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	out := domain.Page[domain.AuditLog]{Items: []domain.AuditLog{}, Page: page, PageSize: limit}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_logs`+clause, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, email, event, ip, device, location, detail, created_at FROM audit_logs`+clause+
			` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var l domain.AuditLog
		var uid sql.NullInt64
		var created int64
		if err := rows.Scan(&l.ID, &uid, &l.Email, &l.Event, &l.IP, &l.Device, &l.Location, &l.Detail, &created); err != nil {
			return out, err
		}
		if uid.Valid {
			l.UserID = &uid.Int64
		}
		l.CreatedAt = fromUnix(created)
		out.Items = append(out.Items, l)
	}
	return out, rows.Err()
}
