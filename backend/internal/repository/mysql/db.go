// Package mysql implements the domain repository ports on MySQL 8 (InnoDB)
// through github.com/go-sql-driver/mysql.
package mysql

import (
	"context"
	"database/sql"
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"time"

	"receh/internal/domain"

	driver "github.com/go-sql-driver/mysql"
)

//go:embed schema.sql
var schema string

// MySQL error numbers this package reacts to.
const (
	errDupEntry    = 1062
	errUnknownDB   = 1049
	errDeadlock    = 1213
	errLockTimeout = 1205
)

// Options tune the connection pool.
type Options struct {
	MaxOpenConns int
}

// Open connects, creates the database when it does not exist yet (handy on a
// fresh dev machine), and applies the schema.
//
// The DSN is forced to loc=UTC and parseTime=false: instants are stored as unix
// seconds and DATE columns come back as "2006-01-02" strings, exactly the form
// the domain uses.
func Open(dsn string, opts Options) (*sql.DB, error) {
	cfg, err := driver.ParseDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse DB DSN: %w", err)
	}
	if cfg.DBName == "" {
		return nil, errors.New("DB DSN has no database name")
	}
	cfg.ParseTime = false
	cfg.Loc = time.UTC
	if cfg.Params == nil {
		cfg.Params = map[string]string{}
	}
	cfg.Params["charset"] = "utf8mb4"
	cfg.Collation = "utf8mb4_0900_ai_ci"

	db, err := connect(cfg, opts)
	var me *driver.MySQLError
	if errors.As(err, &me) && me.Number == errUnknownDB {
		if err := createDatabase(*cfg); err != nil {
			return nil, err
		}
		db, err = connect(cfg, opts)
	}
	if err != nil {
		return nil, err
	}
	if err := applySchema(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

func connect(cfg *driver.Config, opts Options) (*sql.DB, error) {
	connector, err := driver.NewConnector(cfg)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db := sql.OpenDB(connector)
	if opts.MaxOpenConns <= 0 {
		opts.MaxOpenConns = 20
	}
	db.SetMaxOpenConns(opts.MaxOpenConns)
	db.SetMaxIdleConns(opts.MaxOpenConns)
	// Below MySQL's default wait_timeout, so the pool never hands out a socket
	// the server already closed.
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to MySQL at %s: %w", cfg.Addr, err)
	}
	return db, nil
}

func createDatabase(cfg driver.Config) error {
	name := cfg.DBName
	if strings.ContainsAny(name, "`\\") {
		return fmt.Errorf("invalid database name %q", name)
	}
	cfg.DBName = ""
	db, err := connect(&cfg, Options{MaxOpenConns: 1})
	if err != nil {
		return err
	}
	defer db.Close()
	_, err = db.Exec("CREATE DATABASE IF NOT EXISTS `" + name + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci")
	if err != nil {
		return fmt.Errorf("create database %s: %w", name, err)
	}
	return nil
}

// applySchema runs schema.sql one statement at a time, so the DSN never needs
// multiStatements (which would widen the blast radius of any injection bug).
func applySchema(db *sql.DB) error {
	for _, stmt := range splitStatements(schema) {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("apply schema: %w\n%s", err, stmt)
		}
	}
	return migrate(db)
}

// migrations add what CREATE TABLE IF NOT EXISTS cannot: columns introduced
// after a database was first created. Each runs only if its column is missing.
var migrations = []struct{ table, column, ddl string }{
	{"users", "google_sub", "ALTER TABLE users ADD COLUMN google_sub VARCHAR(64) NULL AFTER password_hash, ADD UNIQUE KEY uq_users_google (google_sub)"},
}

func migrate(db *sql.DB) error {
	for _, m := range migrations {
		var n int
		err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
			WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = ? AND COLUMN_NAME = ?`, m.table, m.column).Scan(&n)
		if err != nil {
			return fmt.Errorf("check column %s.%s: %w", m.table, m.column, err)
		}
		if n > 0 {
			continue
		}
		if _, err := db.Exec(m.ddl); err != nil {
			return fmt.Errorf("migrate %s.%s: %w", m.table, m.column, err)
		}
	}
	return nil
}

func splitStatements(src string) []string {
	var out []string
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--") || trimmed == "" {
			continue
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if strings.HasSuffix(trimmed, ";") {
			out = append(out, strings.TrimSuffix(strings.TrimSpace(b.String()), ";"))
			b.Reset()
		}
	}
	if s := strings.TrimSpace(b.String()); s != "" {
		out = append(out, s)
	}
	return out
}

// inTx runs fn in a transaction, rolling back on any error. InnoDB may pick a
// transaction as a deadlock victim; that is safe to retry because fn starts
// from scratch each time.
func inTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = runTx(ctx, db, fn)
		if !isRetryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 20 * time.Millisecond):
		}
	}
	return err
}

func runTx(ctx context.Context, db *sql.DB, fn func(*sql.Tx) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck // Rollback after commit is a no-op.
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func mysqlErr(err error, numbers ...uint16) bool {
	var me *driver.MySQLError
	if !errors.As(err, &me) {
		return false
	}
	for _, n := range numbers {
		if me.Number == n {
			return true
		}
	}
	return false
}

func isRetryable(err error) bool { return mysqlErr(err, errDeadlock, errLockTimeout) }

func isUnique(err error) bool { return mysqlErr(err, errDupEntry) }

func mapNoRows(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return domain.ErrNoRows
	}
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func pageBounds(page, size int) (limit, offset, normalizedPage int) {
	if size <= 0 || size > 100 {
		size = 20
	}
	if page < 1 {
		page = 1
	}
	return size, (page - 1) * size, page
}

// escapeLike makes user input literal inside a LIKE pattern. Backslash is
// MySQL's default LIKE escape character, so no ESCAPE clause is needed.
func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func unix(t time.Time) int64 { return t.Unix() }

func fromUnix(v int64) time.Time { return time.Unix(v, 0).UTC() }
