package mysql

import (
	"context"
	"database/sql"
	"time"

	"receh/internal/domain"
)

type SessionRepo struct{ db *sql.DB }

func NewSessionRepo(db *sql.DB) *SessionRepo { return &SessionRepo{db} }

const sessionCols = `id, user_id, token_hash, ip, user_agent, device, location, created_at, last_seen_at, expires_at`

func scanSession(row interface{ Scan(...any) error }) (*domain.Session, error) {
	var s domain.Session
	var created, seen, expires int64
	if err := row.Scan(&s.ID, &s.UserID, &s.TokenHash, &s.IP, &s.UserAgent, &s.Device, &s.Location, &created, &seen, &expires); err != nil {
		return nil, mapNoRows(err)
	}
	s.CreatedAt, s.LastSeenAt, s.ExpiresAt = fromUnix(created), fromUnix(seen), fromUnix(expires)
	return &s, nil
}

func (r *SessionRepo) Create(ctx context.Context, s *domain.Session) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO sessions (user_id, token_hash, ip, user_agent, device, location, created_at, last_seen_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		s.UserID, s.TokenHash, s.IP, s.UserAgent, s.Device, s.Location, unix(s.CreatedAt), unix(s.LastSeenAt), unix(s.ExpiresAt))
	if err != nil {
		return err
	}
	s.ID, err = res.LastInsertId()
	return err
}

func (r *SessionRepo) ByTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	return scanSession(r.db.QueryRowContext(ctx,
		`SELECT `+sessionCols+` FROM sessions WHERE token_hash = ? AND expires_at > ?`, hash, unix(time.Now())))
}

func (r *SessionRepo) ListActive(ctx context.Context, userID int64) ([]domain.Session, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+sessionCols+` FROM sessions WHERE user_id = ? AND expires_at > ? ORDER BY last_seen_at DESC, id DESC`,
		userID, unix(time.Now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Session{}
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, rows.Err()
}

// Revoke deletes the session only if it belongs to userID.
func (r *SessionRepo) Revoke(ctx context.Context, userID, sessionID int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ? AND user_id = ?`, sessionID, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *SessionRepo) RevokeOthers(ctx context.Context, userID, keepID int64) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keepID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (r *SessionRepo) RevokeAll(ctx context.Context, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// Touch writes a batch of last-seen times in one commit.
func (r *SessionRepo) Touch(ctx context.Context, lastSeen map[int64]time.Time) error {
	if len(lastSeen) == 0 {
		return nil
	}
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		for id, t := range lastSeen {
			if _, err := tx.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE id = ?`, unix(t), id); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *SessionRepo) SetLocation(ctx context.Context, sessionID int64, location string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE sessions SET location = ? WHERE id = ?`, location, sessionID)
	return err
}

func (r *SessionRepo) DeleteExpired(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, unix(time.Now()))
	return err
}
