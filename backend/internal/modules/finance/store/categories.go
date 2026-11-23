package store

import (
	"context"
	"database/sql"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
)

type CategoryRepo struct{ db *sql.DB }

func NewCategoryRepo(db *sql.DB) *CategoryRepo { return &CategoryRepo{db} }

var errCategoryName = domain.Conflict("CATEGORY_EXISTS", "Kategori dengan nama ini sudah ada.")

func (r *CategoryRepo) Create(ctx context.Context, c *model.Category) error {
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

func (r *CategoryRepo) List(ctx context.Context, userID int64) ([]model.Category, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, user_id, name, type, color FROM categories WHERE user_id = ? ORDER BY type, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []model.Category{}
	for rows.Next() {
		var c model.Category
		if err := rows.Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Color); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *CategoryRepo) ByID(ctx context.Context, userID, id int64) (*model.Category, error) {
	var c model.Category
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, type, color FROM categories WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Color)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &c, nil
}

// ByName matches case-insensitively through the table collation.
func (r *CategoryRepo) ByName(ctx context.Context, userID int64, t model.TxType, name string) (*model.Category, error) {
	var c model.Category
	err := r.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, type, color FROM categories WHERE user_id = ? AND type = ? AND name = ?`, userID, t, name).
		Scan(&c.ID, &c.UserID, &c.Name, &c.Type, &c.Color)
	if err != nil {
		return nil, mapNoRows(err)
	}
	return &c, nil
}

func (r *CategoryRepo) Update(ctx context.Context, c *model.Category) error {
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

// CreateDefaults gives a new account its starter categories in one commit.
func (r *CategoryRepo) CreateDefaults(ctx context.Context, userID int64, defaults []model.Category) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		for _, c := range defaults {
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO categories (user_id, name, type, color) VALUES (?, ?, ?, ?)`,
				userID, c.Name, c.Type, c.Color); err != nil {
				return err
			}
		}
		return nil
	})
}
