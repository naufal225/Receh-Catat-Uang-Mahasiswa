// Package store is the finance module's MySQL persistence. It reuses the
// core's conventions through the exported helpers of repository/mysql.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

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
	if err := carryOverNotificationPrefs(db); err != nil {
		return err
	}
	return moveToWorkspaces(db)
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

// orgTables lists the tables that used to belong to a user and now belong to a
// workspace. fk/dropIdx name what the old user_id column carried; addIdx is the
// replacement keyed by org_id.
var orgTables = []struct {
	table, oldFK, newFK, dropIdx, newIdx, addIdx string
}{
	{"wallets", "fk_wallets_user", "fk_wallets_org", "uq_wallet_name", "uq_wallet_name", "ADD UNIQUE KEY uq_wallet_name (org_id, active_name)"},
	{"categories", "fk_categories_user", "fk_categories_org", "uq_category_name", "uq_category_name", "ADD UNIQUE KEY uq_category_name (org_id, type, name)"},
	{"recurring", "fk_recurring_user", "fk_recurring_org", "", "", ""},
	{"transfers", "fk_transfers_user", "fk_transfers_org", "idx_transfers_route", "idx_transfers_route", "ADD KEY idx_transfers_route (org_id, from_wallet_id, to_wallet_id, id)"},
	{"transactions", "fk_tx_user", "fk_tx_org", "idx_tx_user_date", "idx_tx_org_date", "ADD KEY idx_tx_org_date (org_id, occurred_on DESC, id DESC)"},
	{"budgets", "fk_budget_user", "fk_budget_org", "uq_budget_category", "uq_budget_category", "ADD UNIQUE KEY uq_budget_category (org_id, category_id)"},
	{"goals", "fk_goal_user", "fk_goal_org", "idx_goals_user", "idx_goals_org", "ADD KEY idx_goals_org (org_id, archived_at)"},
	{"import_batches", "fk_import_user", "fk_import_org", "idx_import_user", "idx_import_org", "ADD KEY idx_import_org (org_id, created_at)"},
}

// moveToWorkspaces converts a database from before organizations existed: rows
// that belonged to a user now belong to that user's personal workspace.
//
// It never deletes data. user_id is only renamed to legacy_user_id (nullable,
// no longer constrained), so the original owner of every row stays on record.
// Every step checks the schema first, so a run that stopped halfway (or a
// second run) continues where it left off. Back up before the first run on a
// real database anyway: DDL in MySQL cannot be rolled back.
func moveToWorkspaces(db *sql.DB) error {
	for _, t := range orgTables {
		hasUser, err := core.ColumnExists(db, t.table, "user_id")
		if err != nil {
			return err
		}
		if !hasUser {
			continue // fresh database, or already converted
		}
		step := func(ddl string) error {
			if _, err := db.Exec(ddl); err != nil {
				return fmt.Errorf("move %s to workspaces: %w\n%s", t.table, err, ddl)
			}
			return nil
		}
		if has, err := core.ColumnExists(db, t.table, "org_id"); err != nil {
			return err
		} else if !has {
			if err := step("ALTER TABLE " + t.table + " ADD COLUMN org_id BIGINT NULL"); err != nil {
				return err
			}
		}
		// Rows of an account without a workspace (an old admin account that once
		// received starter data) get one too, so nothing is left behind.
		rows, err := db.Query("SELECT DISTINCT user_id FROM " + t.table + " WHERE org_id IS NULL")
		if err != nil {
			return err
		}
		var owners []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			owners = append(owners, id)
		}
		rows.Close()
		for _, id := range owners {
			if err := core.EnsurePersonalOrg(db, id); err != nil {
				return err
			}
		}
		// Each user's rows go to their personal workspace. Only rows not yet
		// moved are touched.
		if err := step(`UPDATE ` + t.table + ` t JOIN organizations o ON o.owner_id = t.user_id AND o.personal = 1
			SET t.org_id = o.id WHERE t.org_id IS NULL`); err != nil {
			return err
		}
		var orphans int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + t.table + " WHERE org_id IS NULL").Scan(&orphans); err != nil {
			return err
		}
		if orphans > 0 {
			return fmt.Errorf("move %s to workspaces: %d rows belong to a user without a personal workspace; nothing was removed", t.table, orphans)
		}
		if ok, err := core.ConstraintExists(db, t.table, t.oldFK); err != nil {
			return err
		} else if ok {
			if err := step("ALTER TABLE " + t.table + " DROP FOREIGN KEY " + t.oldFK); err != nil {
				return err
			}
		}
		if t.dropIdx != "" {
			if ok, err := core.IndexExists(db, t.table, t.dropIdx); err != nil {
				return err
			} else if ok {
				if err := step("ALTER TABLE " + t.table + " DROP INDEX " + t.dropIdx); err != nil {
					return err
				}
			}
		}
		if t.addIdx != "" {
			if ok, err := core.IndexExists(db, t.table, t.newIdx); err != nil {
				return err
			} else if !ok {
				if err := step("ALTER TABLE " + t.table + " " + t.addIdx); err != nil {
					return err
				}
			}
		}
		if err := step("ALTER TABLE " + t.table + " MODIFY org_id BIGINT NOT NULL"); err != nil {
			return err
		}
		if ok, err := core.ConstraintExists(db, t.table, t.newFK); err != nil {
			return err
		} else if !ok {
			if err := step("ALTER TABLE " + t.table + " ADD CONSTRAINT " + t.newFK +
				" FOREIGN KEY (org_id) REFERENCES organizations (id) ON DELETE CASCADE"); err != nil {
				return err
			}
		}
		// Last step: once user_id is renamed this table counts as converted.
		if err := step("ALTER TABLE " + t.table + " CHANGE user_id legacy_user_id BIGINT NULL"); err != nil {
			return err
		}
	}
	return nil
}
