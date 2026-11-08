package mysql

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"receh/internal/domain"
)

type TransferRepo struct{ db *sql.DB }

func NewTransferRepo(db *sql.DB) *TransferRepo { return &TransferRepo{db} }

// effects is the balance change a transfer makes on each wallet it touches.
func effects(t *domain.Transfer, sign int64) map[int64]int64 {
	return map[int64]int64{
		t.FromWalletID: -t.Total() * sign,
		t.ToWalletID:   t.Amount * sign,
	}
}

// applyEffects nets the changes per wallet and applies them in ascending wallet
// id. Netting means an edit that keeps the same source only checks the
// difference against its balance; the fixed order means two transactions over
// the same pair of wallets take InnoDB row locks in the same sequence.
func applyEffects(ctx context.Context, tx *sql.Tx, parts ...map[int64]int64) error {
	net := map[int64]int64{}
	for _, p := range parts {
		for id, d := range p {
			net[id] += d
		}
	}
	ids := make([]int64, 0, len(net))
	for id := range net {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		if err := applyDelta(ctx, tx, id, net[id]); err != nil {
			return err
		}
	}
	return nil
}

// insertLegs writes the rows that make a transfer visible in history: one leg
// per wallet and, when there is a fee, an ordinary expense so the fee shows up
// in charts and reports like any other spending.
func insertLegs(ctx context.Context, tx *sql.Tx, t *domain.Transfer) error {
	id := t.ID
	name := t.Note
	if name == "" {
		name = "Transfer"
	}
	legs := []domain.Transaction{
		{WalletID: t.FromWalletID, Type: domain.TxTransferOut, Amount: t.Amount, Name: name},
		{WalletID: t.ToWalletID, Type: domain.TxTransferIn, Amount: t.Amount, Name: name},
	}
	if t.Fee > 0 {
		legs = append(legs, domain.Transaction{
			WalletID: t.FromWalletID, Type: domain.TxExpense, Amount: t.Fee, CategoryID: t.FeeCategoryID,
			Name: "Biaya transfer " + t.FromWalletName + " ke " + t.ToWalletName,
		})
	}
	for i := range legs {
		legs[i].UserID, legs[i].OccurredOn, legs[i].TransferID = t.UserID, t.OccurredOn, &id
		if err := insertTx(ctx, tx, &legs[i]); err != nil {
			return err
		}
	}
	return nil
}

func (r *TransferRepo) Create(ctx context.Context, t *domain.Transfer) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := applyEffects(ctx, tx, effects(t, 1)); err != nil {
			return err
		}
		t.CreatedAt = time.Now().UTC()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO transfers (user_id, from_wallet_id, to_wallet_id, amount, fee, note, occurred_on, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			t.UserID, t.FromWalletID, t.ToWalletID, t.Amount, t.Fee, t.Note, t.OccurredOn, unix(t.CreatedAt))
		if err != nil {
			return err
		}
		if t.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		return insertLegs(ctx, tx, t)
	})
}

// Update reverses the old transfer and applies the new one in one commit; the
// legs are rewritten rather than patched, since wallets and fee may all change.
func (r *TransferRepo) Update(ctx context.Context, old, updated *domain.Transfer) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := applyEffects(ctx, tx, effects(old, -1), effects(updated, 1)); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE transfers SET from_wallet_id = ?, to_wallet_id = ?, amount = ?, fee = ?, note = ?, occurred_on = ?
			 WHERE id = ? AND user_id = ?`,
			updated.FromWalletID, updated.ToWalletID, updated.Amount, updated.Fee, updated.Note, updated.OccurredOn,
			old.ID, old.UserID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			// Nothing to change is fine (MySQL reports 0 for identical values),
			// but a vanished row is not.
			var exists int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM transfers WHERE id = ? AND user_id = ?`, old.ID, old.UserID).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				return domain.ErrNoRows
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM transactions WHERE transfer_id = ?`, old.ID); err != nil {
			return err
		}
		updated.ID, updated.UserID = old.ID, old.UserID
		return insertLegs(ctx, tx, updated)
	})
}

func (r *TransferRepo) Delete(ctx context.Context, t *domain.Transfer) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		if err := applyEffects(ctx, tx, effects(t, -1)); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM transactions WHERE transfer_id = ?`, t.ID); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `DELETE FROM transfers WHERE id = ? AND user_id = ?`, t.ID, t.UserID)
		return err
	})
}

func (r *TransferRepo) ByID(ctx context.Context, userID, id int64) (*domain.Transfer, error) {
	var t domain.Transfer
	var created int64
	err := r.db.QueryRowContext(ctx, `
		SELECT tr.id, tr.user_id, tr.from_wallet_id, fw.name, tr.to_wallet_id, tw.name,
		       tr.amount, tr.fee, tr.note, tr.occurred_on, tr.created_at
		FROM transfers tr
		JOIN wallets fw ON fw.id = tr.from_wallet_id
		JOIN wallets tw ON tw.id = tr.to_wallet_id
		WHERE tr.id = ? AND tr.user_id = ?`, id, userID).
		Scan(&t.ID, &t.UserID, &t.FromWalletID, &t.FromWalletName, &t.ToWalletID, &t.ToWalletName,
			&t.Amount, &t.Fee, &t.Note, &t.OccurredOn, &created)
	if err != nil {
		return nil, mapNoRows(err)
	}
	t.CreatedAt = fromUnix(created)
	return &t, nil
}

func (r *TransferRepo) LastFee(ctx context.Context, userID, fromWalletID, toWalletID int64) (int64, bool, error) {
	var fee int64
	err := r.db.QueryRowContext(ctx,
		`SELECT fee FROM transfers WHERE user_id = ? AND from_wallet_id = ? AND to_wallet_id = ? ORDER BY id DESC LIMIT 1`,
		userID, fromWalletID, toWalletID).Scan(&fee)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	return fee, true, nil
}
