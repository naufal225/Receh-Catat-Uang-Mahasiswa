package store

import (
	"context"
	"database/sql"
	"time"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
)

type WalletRepo struct{ db *sql.DB }

func NewWalletRepo(db *sql.DB) *WalletRepo { return &WalletRepo{db} }

const walletCols = `id, org_id, name, type, color, balance, archived`

func scanWallet(row interface{ Scan(...any) error }) (*model.Wallet, error) {
	var w model.Wallet
	var archived int
	if err := row.Scan(&w.ID, &w.OrgID, &w.Name, &w.Type, &w.Color, &w.Balance, &archived); err != nil {
		return nil, mapNoRows(err)
	}
	w.Archived = archived == 1
	return &w, nil
}

var errWalletName = domain.Conflict("WALLET_EXISTS", "Anda sudah punya dompet dengan nama ini.")

func (r *WalletRepo) Create(ctx context.Context, w *model.Wallet) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO wallets (org_id, name, type, color, balance, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		w.OrgID, w.Name, w.Type, w.Color, w.Balance, unix(time.Now()))
	if isUnique(err) {
		return errWalletName
	}
	if err != nil {
		return err
	}
	w.ID, err = res.LastInsertId()
	return err
}

func (r *WalletRepo) List(ctx context.Context, orgID int64, includeArchived bool) ([]model.Wallet, error) {
	q := `SELECT ` + walletCols + ` FROM wallets WHERE org_id = ?`
	if !includeArchived {
		q += ` AND archived = 0`
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY archived, balance DESC, id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Wallet{}
	for rows.Next() {
		w, err := scanWallet(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func (r *WalletRepo) ByID(ctx context.Context, orgID, id int64) (*model.Wallet, error) {
	return scanWallet(r.db.QueryRowContext(ctx,
		`SELECT `+walletCols+` FROM wallets WHERE id = ? AND org_id = ?`, id, orgID))
}

// Update never touches balance: only transactions may move money.
func (r *WalletRepo) Update(ctx context.Context, w *model.Wallet) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE wallets SET name = ?, type = ?, color = ? WHERE id = ? AND org_id = ?`,
		w.Name, w.Type, w.Color, w.ID, w.OrgID)
	if isUnique(err) {
		return errWalletName
	}
	return err
}

func (r *WalletRepo) HasTransactions(ctx context.Context, id int64) (bool, error) {
	var found int
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM transactions WHERE wallet_id = ?)
		     OR EXISTS (SELECT 1 FROM recurring WHERE wallet_id = ?)`, id, id).Scan(&found)
	return found == 1, err
}

func (r *WalletRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM wallets WHERE id = ?`, id)
	return err
}

func (r *WalletRepo) Archive(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE wallets SET archived = 1 WHERE id = ?`, id)
	return err
}
