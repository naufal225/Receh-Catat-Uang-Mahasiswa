package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"receh/internal/domain"
)

type UserRepo struct{ db *sql.DB }

func NewUserRepo(db *sql.DB) *UserRepo { return &UserRepo{db} }

const userCols = `id, name, email, password_hash, COALESCE(google_sub, ''), role, status, status_reason, created_at`

func scanUser(row interface{ Scan(...any) error }) (*domain.User, error) {
	var u domain.User
	var created int64
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.PasswordHash, &u.GoogleSub, &u.Role, &u.Status, &u.StatusReason, &created); err != nil {
		return nil, mapNoRows(err)
	}
	u.CreatedAt = fromUnix(created)
	u.HasPassword, u.GoogleLinked = u.PasswordHash != "", u.GoogleSub != ""
	return &u, nil
}

func insertUser(ctx context.Context, x interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, u *domain.User) error {
	u.CreatedAt = time.Now().UTC()
	res, err := x.ExecContext(ctx,
		`INSERT INTO users (name, email, password_hash, google_sub, role, status, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.Name, u.Email, u.PasswordHash, nullString(u.GoogleSub), u.Role, u.Status, unix(u.CreatedAt))
	if isUnique(err) {
		return domain.Conflict("EMAIL_TAKEN", "Email ini sudah terdaftar.")
	}
	if err != nil {
		return err
	}
	u.ID, err = res.LastInsertId()
	return err
}

func (r *UserRepo) Create(ctx context.Context, u *domain.User) error {
	return insertUser(ctx, r.db, u)
}

// CreateWithDefaults creates the user and their starter categories in one
// commit, so an account never exists without its categories.
func (r *UserRepo) CreateWithDefaults(ctx context.Context, u *domain.User, defaults []domain.Category) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := insertUser(ctx, tx, u); err != nil {
			return err
		}
		for _, c := range defaults {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO categories (user_id, name, type, color) VALUES (?, ?, ?, ?)`,
				u.ID, c.Name, c.Type, c.Color); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *UserRepo) ByID(ctx context.Context, id int64) (*domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (r *UserRepo) ByEmail(ctx context.Context, email string) (*domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, email))
}

func (r *UserRepo) ByGoogleSub(ctx context.Context, sub string) (*domain.User, error) {
	return scanUser(r.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE google_sub = ?`, sub))
}

// LinkGoogle never overwrites: an account already linked to another Google
// account, or a Google account already used elsewhere, is refused.
func (r *UserRepo) LinkGoogle(ctx context.Context, id int64, sub string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE users SET google_sub = ? WHERE id = ? AND google_sub IS NULL`, sub, id)
	if isUnique(err) {
		return domain.ErrOAuthFailed
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return domain.ErrOAuthFailed
	}
	return nil
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id int64, hash string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	return err
}

func (r *UserRepo) UpdateStatus(ctx context.Context, id int64, status domain.UserStatus, reason string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE users SET status = ?, status_reason = ? WHERE id = ?`, status, reason, id)
	return err
}

func (r *UserRepo) List(ctx context.Context, f domain.UserFilter) (domain.Page[domain.User], error) {
	limit, offset, page := pageBounds(f.Page, f.PageSize)
	var where []string
	var args []any
	if s := strings.TrimSpace(f.Search); s != "" {
		like := "%" + escapeLike(s) + "%"
		where = append(where, `(name LIKE ? OR email LIKE ?)`)
		args = append(args, like, like)
	}
	if f.Status != "" {
		where = append(where, `status = ?`)
		args = append(args, f.Status)
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}
	out := domain.Page[domain.User]{Items: []domain.User{}, Page: page, PageSize: limit}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`+clause, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+userCols+` FROM users`+clause+` ORDER BY created_at DESC, id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, *u)
	}
	return out, rows.Err()
}

func (r *UserRepo) CountAdmins(ctx context.Context) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin'`).Scan(&n)
	return n, err
}

func (r *UserRepo) ActiveUserIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM users WHERE role = 'user' AND status = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}
