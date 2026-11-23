// Package store is the finance module's MySQL persistence. It reuses the
// core's conventions through the exported helpers of repository/mysql.
package store

import (
	"database/sql"
	_ "embed"

	core "receh/internal/repository/mysql"
)

//go:embed schema.sql
var schema string

var migrations = []core.Migration{
	{Table: "transactions", Column: "import_batch_id", DDL: "ALTER TABLE transactions ADD COLUMN import_batch_id BIGINT NULL AFTER transfer_id, ADD KEY idx_tx_import (import_batch_id)"},
}

// Migrate creates the module's tables (after the core ones: they reference
// users) and applies its column migrations.
func Migrate(db *sql.DB) error {
	if err := core.ApplySchema(db, schema); err != nil {
		return err
	}
	if err := core.Migrate(db, migrations); err != nil {
		return err
	}
	return carryOverNotificationPrefs(db)
}

// carryOverNotificationPrefs moves settings from the columns notification_prefs
// had before modules existed into its JSON settings, once per row.
func carryOverNotificationPrefs(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM information_schema.COLUMNS
		WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'notification_prefs' AND COLUMN_NAME = 'low_balance_threshold'`).Scan(&n); err != nil || n == 0 {
		return err
	}
	_, err := db.Exec(`UPDATE notification_prefs
		SET settings = JSON_OBJECT('finance.low_balance_threshold', low_balance_threshold, 'finance.reminder_hour', reminder_hour)
		WHERE settings IS NULL`)
	return err
}

var (
	inTx       = core.InTx
	isUnique   = core.IsUnique
	mapNoRows  = core.MapNoRows
	pageBounds = core.PageBounds
	escapeLike = core.EscapeLike
	unix       = core.Unix
	fromUnix   = core.FromUnix
	boolInt    = core.BoolInt
	nullTime   = core.NullTime
)
