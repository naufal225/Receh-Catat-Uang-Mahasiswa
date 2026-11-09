package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"receh/internal/domain"
)

type TransactionRepo struct{ db *sql.DB }

func NewTransactionRepo(db *sql.DB) *TransactionRepo { return &TransactionRepo{db} }

// txSelect joins everything a row needs to render: its wallet, its category
// (absent on transfer legs) and, for a transfer leg, the wallet on the other
// side plus the fee the source paid.
const txSelect = `
	SELECT t.id, t.user_id, t.wallet_id, w.name, w.color,
	       COALESCE(t.category_id, 0), COALESCE(c.name, ''), COALESCE(c.color, ''),
	       t.type, t.name, t.amount, t.occurred_on, t.recurring_id, t.transfer_id, t.import_batch_id,
	       COALESCE(cw.name, ''), COALESCE(cw.color, ''),
	       CASE WHEN t.type = 'transfer_out' THEN COALESCE(tr.fee, 0) ELSE 0 END,
	       t.created_at
	FROM transactions t
	JOIN wallets w ON w.id = t.wallet_id
	LEFT JOIN categories c ON c.id = t.category_id
	LEFT JOIN transfers tr ON tr.id = t.transfer_id
	LEFT JOIN wallets cw ON cw.id = CASE t.type
	    WHEN 'transfer_out' THEN tr.to_wallet_id
	    WHEN 'transfer_in' THEN tr.from_wallet_id END`

func scanTx(row interface{ Scan(...any) error }) (*domain.Transaction, error) {
	var t domain.Transaction
	var recurring, transfer, batch sql.NullInt64
	var created int64
	err := row.Scan(&t.ID, &t.UserID, &t.WalletID, &t.WalletName, &t.WalletColor, &t.CategoryID, &t.CategoryName,
		&t.CategoryColor, &t.Type, &t.Name, &t.Amount, &t.OccurredOn, &recurring, &transfer, &batch,
		&t.CounterpartName, &t.CounterpartColor, &t.TransferFee, &created)
	if err != nil {
		return nil, mapNoRows(err)
	}
	if recurring.Valid {
		t.RecurringID = &recurring.Int64
	}
	if transfer.Valid {
		t.TransferID = &transfer.Int64
	}
	if batch.Valid {
		t.ImportBatchID = &batch.Int64
	}
	t.CreatedAt = fromUnix(created)
	return &t, nil
}

// applyDelta moves a wallet balance. The WHERE clause refuses any change that
// would take the balance below zero, even if a caller skipped the mutex, and
// InnoDB holds the row lock until commit.
func applyDelta(ctx context.Context, tx *sql.Tx, walletID, delta int64) error {
	if delta == 0 {
		return nil
	}
	res, err := tx.ExecContext(ctx,
		`UPDATE wallets SET balance = balance + ? WHERE id = ? AND balance + ? >= 0`, delta, walletID, delta)
	if err != nil {
		return err
	}
	// MySQL reports changed rows; delta != 0, so a matched row always changes.
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return domain.ErrInsufficient
	}
	return nil
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

func insertTx(ctx context.Context, tx *sql.Tx, t *domain.Transaction) error {
	t.CreatedAt = time.Now().UTC()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO transactions (user_id, wallet_id, category_id, type, name, amount, occurred_on, recurring_id, transfer_id, import_batch_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.UserID, t.WalletID, nullID(t.CategoryID), t.Type, t.Name, t.Amount, t.OccurredOn, t.RecurringID, nullInt(t.TransferID),
		nullInt(t.ImportBatchID), unix(t.CreatedAt))
	if err != nil {
		return err
	}
	t.ID, err = res.LastInsertId()
	return err
}

func (r *TransactionRepo) Create(ctx context.Context, t *domain.Transaction) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := applyDelta(ctx, tx, t.WalletID, t.Type.Delta(t.Amount)); err != nil {
			return err
		}
		return insertTx(ctx, tx, t)
	})
}

// Update reverses the old effect and applies the new one in a single commit.
func (r *TransactionRepo) Update(ctx context.Context, old, updated *domain.Transaction) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		oldDelta, newDelta := old.Type.Delta(old.Amount), updated.Type.Delta(updated.Amount)
		if old.WalletID == updated.WalletID {
			if err := applyDelta(ctx, tx, old.WalletID, newDelta-oldDelta); err != nil {
				return err
			}
		} else {
			if err := applyDelta(ctx, tx, old.WalletID, -oldDelta); err != nil {
				return err
			}
			if err := applyDelta(ctx, tx, updated.WalletID, newDelta); err != nil {
				return err
			}
		}
		_, err := tx.ExecContext(ctx,
			`UPDATE transactions SET wallet_id = ?, category_id = ?, type = ?, name = ?, amount = ?, occurred_on = ?
			 WHERE id = ? AND user_id = ? AND transfer_id IS NULL`,
			updated.WalletID, updated.CategoryID, updated.Type, updated.Name, updated.Amount, updated.OccurredOn,
			updated.ID, updated.UserID)
		if isUnique(err) {
			return domain.Conflict("DUPLICATE_OCCURRENCE", "Transaksi rutin ini sudah tercatat pada tanggal tersebut.")
		}
		return err
	})
}

func (r *TransactionRepo) Delete(ctx context.Context, t *domain.Transaction) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := applyDelta(ctx, tx, t.WalletID, -t.Type.Delta(t.Amount)); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM transactions WHERE id = ? AND user_id = ? AND transfer_id IS NULL`, t.ID, t.UserID)
		return err
	})
}

func (r *TransactionRepo) ByID(ctx context.Context, userID, id int64) (*domain.Transaction, error) {
	return scanTx(r.db.QueryRowContext(ctx, txSelect+` WHERE t.id = ? AND t.user_id = ?`, id, userID))
}

// visibleLegs keeps one row per transfer in a list across all wallets (the
// outgoing leg, which names both sides). Filtered to one wallet, that wallet's
// own leg shows, so its history explains every balance change.
func visibleLegs(walletID int64) string {
	if walletID > 0 {
		return ""
	}
	return ` AND t.type <> 'transfer_in'`
}

func (r *TransactionRepo) List(ctx context.Context, userID int64, f domain.TxFilter) (domain.Page[domain.Transaction], error) {
	limit, offset, page := pageBounds(f.Page, f.PageSize)
	where := []string{`t.user_id = ?`}
	args := []any{userID}
	if s := strings.TrimSpace(f.Search); s != "" {
		where = append(where, `t.name LIKE ?`)
		args = append(args, "%"+escapeLike(s)+"%")
	}
	switch f.Type {
	case "":
	case domain.TxTransfer:
		where = append(where, `t.type IN ('transfer_out', 'transfer_in')`)
	default:
		where = append(where, `t.type = ?`)
		args = append(args, f.Type)
	}
	if f.WalletID > 0 {
		where = append(where, `t.wallet_id = ?`)
		args = append(args, f.WalletID)
	}
	if f.CategoryID > 0 {
		where = append(where, `t.category_id = ?`)
		args = append(args, f.CategoryID)
	}
	if f.From != "" {
		where = append(where, `t.occurred_on >= ?`)
		args = append(args, f.From)
	}
	if f.To != "" {
		where = append(where, `t.occurred_on <= ?`)
		args = append(args, f.To)
	}
	clause := ` WHERE ` + strings.Join(where, " AND ") + visibleLegs(f.WalletID)

	out := domain.Page[domain.Transaction]{Items: []domain.Transaction{}, Page: page, PageSize: limit}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM transactions t`+clause, args...).Scan(&out.Total); err != nil {
		return out, err
	}
	rows, err := r.db.QueryContext(ctx,
		txSelect+clause+` ORDER BY t.occurred_on DESC, t.id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return out, err
		}
		out.Items = append(out.Items, *t)
	}
	return out, rows.Err()
}

func (r *TransactionRepo) Latest(ctx context.Context, userID, walletID int64, limit int) ([]domain.Transaction, error) {
	q := txSelect + ` WHERE t.user_id = ?`
	args := []any{userID}
	if walletID > 0 {
		q += ` AND t.wallet_id = ?`
		args = append(args, walletID)
	}
	q += visibleLegs(walletID)
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY t.occurred_on DESC, t.id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectTx(rows)
}

func (r *TransactionRepo) AllBetween(ctx context.Context, userID int64, from, to string) ([]domain.Transaction, error) {
	rows, err := r.db.QueryContext(ctx,
		txSelect+` WHERE t.user_id = ? AND t.occurred_on BETWEEN ? AND ?`+visibleLegs(0)+` ORDER BY t.occurred_on, t.id`,
		userID, from, to)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectTx(rows)
}

func collectTx(rows *sql.Rows) ([]domain.Transaction, error) {
	out := []domain.Transaction{}
	for rows.Next() {
		t, err := scanTx(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// SumByDay counts only income and expense: transfer legs fall through both CASE
// arms, so moving money between one's own wallets never inflates the chart.
func (r *TransactionRepo) SumByDay(ctx context.Context, userID, walletID int64, from, to string) ([]domain.DaySum, error) {
	q := `SELECT occurred_on,
	             COALESCE(SUM(CASE WHEN type = 'income' THEN amount ELSE 0 END), 0),
	             COALESCE(SUM(CASE WHEN type = 'expense' THEN amount ELSE 0 END), 0)
	      FROM transactions WHERE user_id = ? AND occurred_on BETWEEN ? AND ?`
	args := []any{userID, from, to}
	if walletID > 0 {
		q += ` AND wallet_id = ?`
		args = append(args, walletID)
	}
	rows, err := r.db.QueryContext(ctx, q+` GROUP BY occurred_on ORDER BY occurred_on`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.DaySum{}
	for rows.Next() {
		var d domain.DaySum
		if err := rows.Scan(&d.Day, &d.Income, &d.Expense); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (r *TransactionRepo) SumByCategory(ctx context.Context, userID, walletID int64, t domain.TxType, from, to string) ([]domain.CategorySlice, error) {
	q := `SELECT c.id, c.name, c.color, SUM(tx.amount) AS total
	      FROM transactions tx JOIN categories c ON c.id = tx.category_id
	      WHERE tx.user_id = ? AND tx.type = ? AND tx.occurred_on BETWEEN ? AND ?`
	args := []any{userID, t, from, to}
	if walletID > 0 {
		q += ` AND tx.wallet_id = ?`
		args = append(args, walletID)
	}
	rows, err := r.db.QueryContext(ctx, q+` GROUP BY c.id ORDER BY total DESC, c.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.CategorySlice{}
	for rows.Next() {
		var s domain.CategorySlice
		if err := rows.Scan(&s.CategoryID, &s.Name, &s.Color, &s.Total); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// WalletSummaries lists each wallet with activity in the range or a live balance.
func (r *TransactionRepo) WalletSummaries(ctx context.Context, userID int64, from, to string) ([]domain.WalletSummary, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT w.name,
		       COALESCE(SUM(CASE WHEN t.type = 'income' THEN t.amount END), 0) AS income,
		       COALESCE(SUM(CASE WHEN t.type = 'expense' THEN t.amount END), 0) AS expense,
		       COALESCE(SUM(CASE WHEN t.type = 'transfer_in' THEN t.amount END), 0) AS moved_in,
		       COALESCE(SUM(CASE WHEN t.type = 'transfer_out' THEN t.amount END), 0) AS moved_out,
		       w.balance
		FROM wallets w
		LEFT JOIN transactions t ON t.wallet_id = w.id AND t.occurred_on BETWEEN ? AND ?
		WHERE w.user_id = ?
		GROUP BY w.id
		HAVING MAX(w.archived) = 0 OR COUNT(t.id) > 0
		ORDER BY w.balance DESC, w.id`, from, to, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.WalletSummary{}
	for rows.Next() {
		var s domain.WalletSummary
		if err := rows.Scan(&s.Name, &s.Income, &s.Expense, &s.TransferIn, &s.TransferOut, &s.Balance); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}
