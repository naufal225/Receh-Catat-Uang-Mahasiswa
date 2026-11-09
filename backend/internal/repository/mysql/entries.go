package mysql

import (
	"context"
	"database/sql"
	"slices"
	"time"

	"receh/internal/domain"
)

type EntryRepo struct{ db *sql.DB }

func NewEntryRepo(db *sql.DB) *EntryRepo { return &EntryRepo{db} }

func (r *EntryRepo) Suggest(ctx context.Context, userID int64, prefix string, exact bool, limit int) ([]domain.TxSuggestion, error) {
	pattern := escapeLike(prefix) + "%"
	cond := `t.name LIKE ?`
	if exact {
		pattern, cond = prefix, `t.name = ?`
	}
	// Group past names, then join each group's latest row for its details.
	rows, err := r.db.QueryContext(ctx, `
		SELECT x.name, x.uses, t.type, COALESCE(t.category_id, 0), t.wallet_id, t.amount
		FROM (
			SELECT t.name, COUNT(*) AS uses, MAX(t.id) AS last_id
			FROM transactions t
			WHERE t.user_id = ? AND t.transfer_id IS NULL AND `+cond+`
			GROUP BY t.name
			ORDER BY uses DESC, last_id DESC
			LIMIT ?
		) x JOIN transactions t ON t.id = x.last_id
		ORDER BY x.uses DESC, x.last_id DESC`, userID, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TxSuggestion{}
	for rows.Next() {
		var s domain.TxSuggestion
		if err := rows.Scan(&s.Name, &s.Uses, &s.Type, &s.CategoryID, &s.WalletID, &s.Amount); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *EntryRepo) ExistingKeys(ctx context.Context, userID, walletID int64, from, to string) (map[string]bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT occurred_on, type, amount, name FROM transactions
		 WHERE user_id = ? AND wallet_id = ? AND occurred_on BETWEEN ? AND ? AND type IN ('income', 'expense')`,
		userID, walletID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var date, name string
		var t domain.TxType
		var amount int64
		if err := rows.Scan(&date, &t, &amount, &name); err != nil {
			return nil, err
		}
		out[domain.DuplicateKey(date, t, amount, name)] = true
	}
	return out, rows.Err()
}

// CreateBatch moves the wallet by the batch's net amount once, so an import
// whose later income covers earlier spending is accepted as a whole.
func (r *EntryRepo) CreateBatch(ctx context.Context, b *domain.ImportBatch, txs []domain.Transaction) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		var net int64
		b.TotalIn, b.TotalOut = 0, 0
		for _, t := range txs {
			net += t.Type.Delta(t.Amount)
			if t.Type == domain.TxIncome {
				b.TotalIn += t.Amount
			} else {
				b.TotalOut += t.Amount
			}
		}
		if err := applyDelta(ctx, tx, b.WalletID, net); err != nil {
			return err
		}
		b.Rows, b.CreatedAt = len(txs), time.Now().UTC()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO import_batches (user_id, wallet_id, file_name, row_count, total_in, total_out, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`, b.UserID, b.WalletID, b.FileName, b.Rows, b.TotalIn, b.TotalOut, unix(b.CreatedAt))
		if err != nil {
			return err
		}
		if b.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		for i := range txs {
			txs[i].ImportBatchID = &b.ID
			if err := insertTx(ctx, tx, &txs[i]); err != nil {
				return err
			}
		}
		b.Remaining = b.Rows
		return nil
	})
}

func (r *EntryRepo) Batches(ctx context.Context, userID int64) ([]domain.ImportBatch, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT b.id, b.user_id, b.wallet_id, w.name, b.file_name, b.row_count, b.total_in, b.total_out, b.created_at,
		       (SELECT COUNT(*) FROM transactions t WHERE t.import_batch_id = b.id)
		FROM import_batches b JOIN wallets w ON w.id = b.wallet_id
		WHERE b.user_id = ? ORDER BY b.created_at DESC, b.id DESC LIMIT 50`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ImportBatch{}
	for rows.Next() {
		var b domain.ImportBatch
		var created int64
		if err := rows.Scan(&b.ID, &b.UserID, &b.WalletID, &b.WalletName, &b.FileName, &b.Rows, &b.TotalIn, &b.TotalOut, &created, &b.Remaining); err != nil {
			return nil, err
		}
		b.CreatedAt = fromUnix(created)
		out = append(out, b)
	}
	return out, rows.Err()
}

// UndoBatch reverses what is left of the batch, per wallet (a row may have
// been moved to another wallet since), and deletes it.
func (r *EntryRepo) UndoBatch(ctx context.Context, userID, batchID int64) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM import_batches WHERE id = ? AND user_id = ?`, batchID, userID).Scan(&exists); err != nil {
			return err
		}
		if exists == 0 {
			return domain.ErrNoRows
		}
		rows, err := tx.QueryContext(ctx, `
			SELECT wallet_id, SUM(CASE WHEN type = 'income' THEN amount ELSE -amount END)
			FROM transactions WHERE import_batch_id = ? AND user_id = ? GROUP BY wallet_id`, batchID, userID)
		if err != nil {
			return err
		}
		nets := map[int64]int64{}
		for rows.Next() {
			var w, net int64
			if err := rows.Scan(&w, &net); err != nil {
				rows.Close()
				return err
			}
			nets[w] = net
		}
		rows.Close()
		ids := make([]int64, 0, len(nets))
		for id := range nets {
			ids = append(ids, id)
		}
		slices.Sort(ids) // same lock order as transfers
		for _, id := range ids {
			if err := applyDelta(ctx, tx, id, -nets[id]); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM transactions WHERE import_batch_id = ? AND user_id = ?`, batchID, userID); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM import_batches WHERE id = ? AND user_id = ?`, batchID, userID)
		return err
	})
}
