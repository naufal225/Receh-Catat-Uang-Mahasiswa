package store

import (
	"context"
	"database/sql"
	"time"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
)

type BudgetRepo struct{ db *sql.DB }

func NewBudgetRepo(db *sql.DB) *BudgetRepo { return &BudgetRepo{db} }

const budgetSelect = `
	SELECT b.id, b.org_id, b.category_id, c.name, c.color, b.amount, b.created_at
	FROM budgets b JOIN categories c ON c.id = b.category_id`

func scanBudget(row interface{ Scan(...any) error }) (*model.Budget, error) {
	var b model.Budget
	var created int64
	if err := row.Scan(&b.ID, &b.OrgID, &b.CategoryID, &b.CategoryName, &b.CategoryColor, &b.Amount, &created); err != nil {
		return nil, mapNoRows(err)
	}
	b.CreatedAt = fromUnix(created)
	return &b, nil
}

func (r *BudgetRepo) Create(ctx context.Context, b *model.Budget) error {
	b.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO budgets (org_id, category_id, amount, created_at) VALUES (?, ?, ?, ?)`,
		b.OrgID, b.CategoryID, b.Amount, unix(b.CreatedAt))
	if isUnique(err) {
		return &domain.AppError{Kind: domain.KindConflict, Code: "BUDGET_EXISTS",
			Message: "Kategori ini sudah punya anggaran. Ubah nominalnya saja.", Field: "categoryId"}
	}
	if err != nil {
		return err
	}
	b.ID, err = res.LastInsertId()
	return err
}

func (r *BudgetRepo) UpdateAmount(ctx context.Context, orgID, id, amount int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE budgets SET amount = ? WHERE id = ? AND org_id = ?`, amount, id, orgID)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	// Same amount reports 0 changed rows; distinguish that from "not found".
	_, err = r.ByID(ctx, orgID, id)
	return err == nil, ignoreNoRows(err)
}

func (r *BudgetRepo) Delete(ctx context.Context, orgID, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM budgets WHERE id = ? AND org_id = ?`, id, orgID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *BudgetRepo) ByID(ctx context.Context, orgID, id int64) (*model.Budget, error) {
	return scanBudget(r.db.QueryRowContext(ctx, budgetSelect+` WHERE b.id = ? AND b.org_id = ?`, id, orgID))
}

func (r *BudgetRepo) List(ctx context.Context, orgID int64) ([]model.Budget, error) {
	rows, err := r.db.QueryContext(ctx, budgetSelect+` WHERE b.org_id = ? ORDER BY c.name, b.id`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Budget{}
	for rows.Next() {
		b, err := scanBudget(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

func (r *BudgetRepo) Count(ctx context.Context, orgID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM budgets WHERE org_id = ?`, orgID).Scan(&n)
	return n, err
}

func ignoreNoRows(err error) error {
	if err == domain.ErrNoRows {
		return nil
	}
	return err
}
