package mysql

import (
	"context"
	"database/sql"

	"receh/internal/domain"
)

type RecurringRepo struct{ db *sql.DB }

func NewRecurringRepo(db *sql.DB) *RecurringRepo { return &RecurringRepo{db} }

const recurringSelect = `
	SELECT r.id, r.user_id, r.wallet_id, w.name, w.color, r.category_id, c.name, c.color,
	       r.type, r.name, r.amount, r.frequency, r.anchor_day, r.next_due, r.active, r.last_error
	FROM recurring r
	JOIN wallets w ON w.id = r.wallet_id
	JOIN categories c ON c.id = r.category_id`

func scanRecurring(row interface{ Scan(...any) error }) (*domain.Recurring, error) {
	var r domain.Recurring
	var active int
	err := row.Scan(&r.ID, &r.UserID, &r.WalletID, &r.WalletName, &r.WalletColor, &r.CategoryID, &r.CategoryName,
		&r.CategoryColor, &r.Type, &r.Name, &r.Amount, &r.Frequency, &r.AnchorDay, &r.NextDue, &active, &r.LastError)
	if err != nil {
		return nil, mapNoRows(err)
	}
	r.Active = active == 1
	return &r, nil
}

func (s *RecurringRepo) Create(ctx context.Context, r *domain.Recurring) error {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO recurring (user_id, wallet_id, category_id, type, name, amount, frequency, anchor_day, next_due, active)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.UserID, r.WalletID, r.CategoryID, r.Type, r.Name, r.Amount, r.Frequency, r.AnchorDay, r.NextDue, boolInt(r.Active))
	if err != nil {
		return err
	}
	r.ID, err = res.LastInsertId()
	return err
}

func (s *RecurringRepo) Update(ctx context.Context, r *domain.Recurring) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE recurring SET wallet_id = ?, category_id = ?, type = ?, name = ?, amount = ?, frequency = ?,
		        anchor_day = ?, next_due = ?, active = ?, last_error = ?
		 WHERE id = ? AND user_id = ?`,
		r.WalletID, r.CategoryID, r.Type, r.Name, r.Amount, r.Frequency, r.AnchorDay, r.NextDue, boolInt(r.Active),
		r.LastError, r.ID, r.UserID)
	return err
}

func (s *RecurringRepo) ByID(ctx context.Context, userID, id int64) (*domain.Recurring, error) {
	return scanRecurring(s.db.QueryRowContext(ctx, recurringSelect+` WHERE r.id = ? AND r.user_id = ?`, id, userID))
}

func (s *RecurringRepo) List(ctx context.Context, userID int64) ([]domain.Recurring, error) {
	rows, err := s.db.QueryContext(ctx,
		recurringSelect+` WHERE r.user_id = ? ORDER BY r.active DESC, r.next_due, r.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRecurring(rows)
}

func (s *RecurringRepo) Delete(ctx context.Context, userID, id int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM recurring WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (s *RecurringRepo) Due(ctx context.Context, day string) ([]domain.Recurring, error) {
	rows, err := s.db.QueryContext(ctx, recurringSelect+`
		JOIN users u ON u.id = r.user_id
		WHERE r.active = 1 AND r.next_due <= ? AND u.status = 'active' AND w.archived = 0
		ORDER BY r.next_due, r.id`, day)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectRecurring(rows)
}

func collectRecurring(rows *sql.Rows) ([]domain.Recurring, error) {
	out := []domain.Recurring{}
	for rows.Next() {
		r, err := scanRecurring(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *r)
	}
	return out, rows.Err()
}

func (s *RecurringRepo) PostOccurrence(ctx context.Context, r *domain.Recurring, on, nextDue string) error {
	return inTx(ctx, s.db, func(tx *sql.Tx) error {
		id := r.ID
		t := &domain.Transaction{
			UserID: r.UserID, WalletID: r.WalletID, CategoryID: r.CategoryID, Type: r.Type,
			Name: r.Name, Amount: r.Amount, OccurredOn: on, RecurringID: &id,
		}
		if err := applyDelta(ctx, tx, r.WalletID, r.Type.Delta(r.Amount)); err != nil {
			return err
		}
		err := insertTx(ctx, tx, t)
		if isUnique(err) {
			// Already posted for this date: undo the balance move and just advance.
			if err := applyDelta(ctx, tx, r.WalletID, -r.Type.Delta(r.Amount)); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE recurring SET next_due = ?, last_error = '' WHERE id = ?`, nextDue, r.ID)
		return err
	})
}

func (s *RecurringRepo) SetLastError(ctx context.Context, id int64, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recurring SET last_error = ? WHERE id = ?`, msg, id)
	return err
}

func (s *RecurringRepo) DeactivateForWallet(ctx context.Context, walletID int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE recurring SET active = 0 WHERE wallet_id = ?`, walletID)
	return err
}
