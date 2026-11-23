package mysql

import (
	"context"
	"database/sql"
	"receh/internal/domain"
	"time"
)

// Exported helpers for module stores, so modules follow the same conventions
// (transactions with deadlock retry, unix-second timestamps, LIKE escaping).

func InTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error { return inTx(ctx, db, fn) }
func IsUnique(err error) bool                                            { return isUnique(err) }
func MapNoRows(err error) error                                          { return mapNoRows(err) }
func PageBounds(page, size int) (limit, offset, normalizedPage int)      { return pageBounds(page, size) }
func EscapeLike(s string) string                                         { return escapeLike(s) }
func Unix(t time.Time) int64                                             { return unix(t) }
func FromUnix(v int64) time.Time                                         { return fromUnix(v) }
func BoolInt(b bool) int                                                 { return boolInt(b) }
func NullTime(t *time.Time) any                                          { return nullTime(t) }

// Schema inspection for migrations, so every step can be guarded and a
// migration that stopped halfway simply continues on the next start.

func exists(db *sql.DB, query string, args ...any) (bool, error) {
	var n int
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

func ColumnExists(db *sql.DB, table, column string) (bool, error) {
	return exists(db, `SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, table, column)
}

func IndexExists(db *sql.DB, table, name string) (bool, error) {
	return exists(db, `SELECT COUNT(*) FROM information_schema.STATISTICS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND INDEX_NAME = ?`, table, name)
}

func ConstraintExists(db *sql.DB, table, name string) (bool, error) {
	return exists(db, `SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND CONSTRAINT_NAME = ?`, table, name)
}

// EnsurePersonalOrg gives any account (even an admin one, whose leftover rows
// from older versions must not be lost) its personal workspace if it has none.
func EnsurePersonalOrg(db *sql.DB, userID int64) error {
	has, err := exists(db, `SELECT COUNT(*) FROM org_members WHERE user_id = ?`, userID)
	if err != nil || has {
		return err
	}
	return NewOrgRepo(db).Create(context.Background(), &domain.Org{Name: PersonalOrgName(""), Personal: true, OwnerID: userID})
}
