package mysql

import (
	"context"
	"database/sql"
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
