package mysql

import (
	"context"
	"database/sql"

	"receh/internal/domain"
)

type CategoryRepo struct{ db *sql.DB }

func NewCategoryRepo(db *sql.DB) *CategoryRepo { return &CategoryRepo{db} }

var errCategoryName = domain.Conflict("CATEGORY_EXISTS", "Kategori dengan nama ini sudah ada.")

func (r *CategoryRepo) Create(ctx context.Context, c *domain.Category) error {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO categories (user_id, name, type, color) VALUES (?, ?, ?, ?)`,
		c.UserID, c.Name, c.Type, c.Color)
	if isUnique(err) {
		return errCategoryName
	}
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

func (r *CategoryRepo) List(ctx context.Context, userID int64) ([]domain.Category, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, type, color FROM categories WHERE user_id = ? ORDER BY type, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Category{}
	for rows.Next() {
		var c domain.Category
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Color); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CategoryRepo) ByID(ctx context.Context, userID, id int64) (*domain.Category, error) {
	var c domain.Category
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, type, color FROM categories WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Color)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &c, nil
}

// ByName matches case-insensitively through the table collation.
func (r *CategoryRepo) ByName(ctx context.Context, userID int64, t domain.TxType, name string) (*domain.Category, error) {
	var c domain.Category
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, type, color FROM categories WHERE user_id = ? AND type = ? AND name = ?`, userID, t, name).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Color)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &c, nil
}

func (r *CategoryRepo) Update(ctx context.Context, c *domain.Category) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE categories SET name = ?, color = ? WHERE id = ? AND user_id = ?`, c.Name, c.Color, c.ID, c.UserID)
	if isUnique(err) {
		return errCategoryName
	}
	return err
}

func (r *CategoryRepo) InUse(ctx context.Context, id int64) (bool, error) {
	var found int
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM transactions WHERE category_id = ?)
		     OR EXISTS (SELECT 1 FROM recurring WHERE category_id = ?)`, id, id).Scan(&found)
	return found == 1, err
}

func (r *CategoryRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM categories WHERE id = ?`, id)
	return err
}
