package mysql

import (
	"context"
	"database/sql"
	"time"

	"receh/internal/domain"
)

type GoalRepo struct{ db *sql.DB }

func NewGoalRepo(db *sql.DB) *GoalRepo { return &GoalRepo{db} }

// goalSelect fills both progress sources in one query: the contribution sum
// and the linked wallet's balance.
const goalSelect = `
	SELECT g.id, g.user_id, g.name, g.target_amount, g.deadline, g.wallet_id, COALESCE(w.name, ''),
	       g.color, g.archived_at, g.created_at,
	       COALESCE((SELECT SUM(c.amount) FROM goal_contributions c WHERE c.goal_id = g.id), 0),
	       COALESCE(w.balance, 0)
	FROM goals g LEFT JOIN wallets w ON w.id = g.wallet_id`

func scanGoal(row interface{ Scan(...any) error }) (*domain.Goal, error) {
	var g domain.Goal
	var deadline sql.NullString
	var wallet, archived sql.NullInt64
	var created int64
	if err := row.Scan(&g.ID, &g.UserID, &g.Name, &g.TargetAmount, &deadline, &wallet, &g.WalletName,
		&g.Color, &archived, &created, &g.Contributed, &g.WalletBalance); err != nil {
		return nil, mapNoRows(err)
	}
	g.CreatedAt = fromUnix(created)
	if deadline.Valid {
		g.Deadline = &deadline.String
	}
	if wallet.Valid {
		g.WalletID = &wallet.Int64
	}
	if archived.Valid {
		t := fromUnix(archived.Int64)
		g.ArchivedAt = &t
	}
	return &g, nil
}

func nullStr(s *string) any {
	if s == nil {
		return nil
	}
	return *s
}

func nullInt(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}

func (r *GoalRepo) Create(ctx context.Context, g *domain.Goal) error {
	g.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO goals (user_id, name, target_amount, deadline, wallet_id, color, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		g.UserID, g.Name, g.TargetAmount, nullStr(g.Deadline), nullInt(g.WalletID), g.Color, unix(g.CreatedAt))
	if err != nil {
		return err
	}
	g.ID, err = res.LastInsertId()
	return err
}

func (r *GoalRepo) Update(ctx context.Context, g *domain.Goal) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE goals SET name = ?, target_amount = ?, deadline = ?, wallet_id = ?, color = ? WHERE id = ? AND user_id = ?`,
		g.Name, g.TargetAmount, nullStr(g.Deadline), nullInt(g.WalletID), g.Color, g.ID, g.UserID)
	return err
}

func (r *GoalRepo) Delete(ctx context.Context, userID, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM goals WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *GoalRepo) ByID(ctx context.Context, userID, id int64) (*domain.Goal, error) {
	return scanGoal(r.db.QueryRowContext(ctx, goalSelect+` WHERE g.id = ? AND g.user_id = ?`, id, userID))
}

func (r *GoalRepo) List(ctx context.Context, userID int64, includeArchived bool) ([]domain.Goal, error) {
	q := goalSelect + ` WHERE g.user_id = ?`
	if !includeArchived {
		q += ` AND g.archived_at IS NULL`
	}
	rows, err := r.db.QueryContext(ctx, q+` ORDER BY g.archived_at IS NOT NULL, g.deadline IS NULL, g.deadline, g.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Goal{}
	for rows.Next() {
		g, err := scanGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *g)
	}
	return out, rows.Err()
}

func (r *GoalRepo) CountActive(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM goals WHERE user_id = ? AND archived_at IS NULL`, userID).Scan(&n)
	return n, err
}

func (r *GoalRepo) SetArchived(ctx context.Context, userID, id int64, at *time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE goals SET archived_at = ? WHERE id = ? AND user_id = ?`, nullTime(at), id, userID)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	_, err = r.ByID(ctx, userID, id)
	return err == nil, ignoreNoRows(err)
}

func (r *GoalRepo) AddContribution(ctx context.Context, c *domain.GoalContribution) error {
	c.CreatedAt = time.Now().UTC()
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO goal_contributions (goal_id, amount, note, occurred_on, created_at) VALUES (?, ?, ?, ?, ?)`,
		c.GoalID, c.Amount, c.Note, c.OccurredOn, unix(c.CreatedAt))
	if err != nil {
		return err
	}
	c.ID, err = res.LastInsertId()
	return err
}

func (r *GoalRepo) Contributions(ctx context.Context, goalID int64) ([]domain.GoalContribution, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, goal_id, amount, note, occurred_on, created_at FROM goal_contributions
		 WHERE goal_id = ? ORDER BY occurred_on DESC, id DESC LIMIT 100`, goalID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.GoalContribution{}
	for rows.Next() {
		var c domain.GoalContribution
		var created int64
		if err := rows.Scan(&c.ID, &c.GoalID, &c.Amount, &c.Note, &c.OccurredOn, &created); err != nil {
			return nil, err
		}
		c.CreatedAt = fromUnix(created)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *GoalRepo) DeleteContribution(ctx context.Context, goalID, id int64) (bool, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM goal_contributions WHERE id = ? AND goal_id = ?`, id, goalID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}
