package mysql

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"receh/internal/domain"
)

type EntitlementRepo struct{ db *sql.DB }

func NewEntitlementRepo(db *sql.DB) *EntitlementRepo { return &EntitlementRepo{db} }

const entSelect = `
	SELECT e.id, e.user_id, e.source, e.plan, e.starts_at, e.ends_at, e.note, e.granted_by,
	       e.promo_code_id, COALESCE(p.code, ''), e.revoked_at, e.created_at
	FROM entitlements e LEFT JOIN promo_codes p ON p.id = e.promo_code_id`

func scanEnt(row interface{ Scan(...any) error }) (*domain.Entitlement, error) {
	var e domain.Entitlement
	var starts, created int64
	var ends, revoked, grantedBy, promoID sql.NullInt64
	if err := row.Scan(&e.ID, &e.UserID, &e.Source, &e.Plan, &starts, &ends, &e.Note, &grantedBy,
		&promoID, &e.PromoCode, &revoked, &created); err != nil {
		return nil, mapNoRows(err)
	}
	e.StartsAt, e.CreatedAt = fromUnix(starts), fromUnix(created)
	if ends.Valid {
		t := fromUnix(ends.Int64)
		e.EndsAt = &t
	}
	if revoked.Valid {
		t := fromUnix(revoked.Int64)
		e.RevokedAt = &t
	}
	if grantedBy.Valid {
		e.GrantedBy = &grantedBy.Int64
	}
	if promoID.Valid {
		e.PromoCodeID = &promoID.Int64
	}
	return &e, nil
}

func nullTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}

func insertEnt(ctx context.Context, x interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, e *domain.Entitlement) error {
	e.CreatedAt = time.Now().UTC()
	res, err := x.ExecContext(ctx,
		`INSERT INTO entitlements (user_id, source, plan, starts_at, ends_at, note, granted_by, promo_code_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.UserID, e.Source, e.Plan, unix(e.StartsAt), nullTime(e.EndsAt), e.Note, e.GrantedBy, e.PromoCodeID, unix(e.CreatedAt))
	if err != nil {
		return err
	}
	e.ID, err = res.LastInsertId()
	return err
}

func (r *EntitlementRepo) Create(ctx context.Context, e *domain.Entitlement) error {
	return insertEnt(ctx, r.db, e)
}

func (r *EntitlementRepo) ActiveFor(ctx context.Context, userID int64, t time.Time) (*domain.Entitlement, error) {
	return scanEnt(r.db.QueryRowContext(ctx, entSelect+`
		WHERE e.user_id = ? AND e.revoked_at IS NULL AND e.starts_at <= ? AND (e.ends_at IS NULL OR e.ends_at > ?)
		ORDER BY e.ends_at IS NULL DESC, e.ends_at DESC, e.id DESC LIMIT 1`, userID, t.Unix(), t.Unix()))
}

func (r *EntitlementRepo) ListForUser(ctx context.Context, userID int64) ([]domain.Entitlement, error) {
	rows, err := r.db.QueryContext(ctx, entSelect+` WHERE e.user_id = ? ORDER BY e.id DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Entitlement{}
	for rows.Next() {
		e, err := scanEnt(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *e)
	}
	return out, rows.Err()
}

func (r *EntitlementRepo) ByID(ctx context.Context, id int64) (*domain.Entitlement, error) {
	return scanEnt(r.db.QueryRowContext(ctx, entSelect+` WHERE e.id = ?`, id))
}

func (r *EntitlementRepo) Revoke(ctx context.Context, id int64, at time.Time) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE entitlements SET revoked_at = ? WHERE id = ? AND revoked_at IS NULL`, at.Unix(), id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *EntitlementRepo) PremiumUserIDs(ctx context.Context, ids []int64, t time.Time) (map[int64]bool, error) {
	out := map[int64]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(ids)+2)
	for _, id := range ids {
		args = append(args, id)
	}
	args = append(args, t.Unix(), t.Unix())
	rows, err := r.db.QueryContext(ctx, `
		SELECT DISTINCT user_id FROM entitlements
		WHERE user_id IN (?`+strings.Repeat(",?", len(ids)-1)+`)
		  AND revoked_at IS NULL AND starts_at <= ? AND (ends_at IS NULL OR ends_at > ?)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ---- Promo codes --------------------------------------------------------------

type PromoRepo struct{ db *sql.DB }

func NewPromoRepo(db *sql.DB) *PromoRepo { return &PromoRepo{db} }

const promoCols = `id, code, plan, duration_days, max_uses, used, expires_at, note, active, COALESCE(created_by, 0), created_at`

func scanPromo(row interface{ Scan(...any) error }) (*domain.PromoCode, error) {
	var p domain.PromoCode
	var duration, expires sql.NullInt64
	var active int
	var created int64
	if err := row.Scan(&p.ID, &p.Code, &p.Plan, &duration, &p.MaxUses, &p.Used, &expires, &p.Note, &active, &p.CreatedBy, &created); err != nil {
		return nil, mapNoRows(err)
	}
	p.Active, p.CreatedAt = active == 1, fromUnix(created)
	if duration.Valid {
		d := int(duration.Int64)
		p.DurationDays = &d
	}
	if expires.Valid {
		t := fromUnix(expires.Int64)
		p.ExpiresAt = &t
	}
	return &p, nil
}

func (r *PromoRepo) Create(ctx context.Context, p *domain.PromoCode) error {
	p.CreatedAt = time.Now().UTC()
	var duration any
	if p.DurationDays != nil {
		duration = *p.DurationDays
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO promo_codes (code, plan, duration_days, max_uses, expires_at, note, active, created_by, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, 1, ?, ?)`,
		p.Code, p.Plan, duration, p.MaxUses, nullTime(p.ExpiresAt), p.Note, p.CreatedBy, unix(p.CreatedAt))
	if isUnique(err) {
		return domain.Conflict("PROMO_EXISTS", "Kode ini sudah ada. Pakai kode lain.")
	}
	if err != nil {
		return err
	}
	p.ID, err = res.LastInsertId()
	p.Active = true
	return err
}

func (r *PromoRepo) List(ctx context.Context) ([]domain.PromoCode, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT `+promoCols+` FROM promo_codes ORDER BY active DESC, id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.PromoCode{}
	for rows.Next() {
		p, err := scanPromo(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *PromoRepo) SetActive(ctx context.Context, id int64, active bool) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE promo_codes SET active = ? WHERE id = ?`, boolInt(active), id)
	if err != nil {
		return false, err
	}
	// Setting the value it already has reports 0 rows changed; check existence.
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	var exists int
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM promo_codes WHERE id = ?`, id).Scan(&exists)
	return exists > 0, err
}

// Redeem locks the code row (SELECT … FOR UPDATE), so parallel redemptions of
// the last use wait in line: exactly max_uses of them succeed. The unique
// (code_id, user_id) index stops one person redeeming twice, and the CHECK on
// used <= max_uses backs both up inside the database.
func (r *PromoRepo) Redeem(ctx context.Context, code string, userID int64, now time.Time) (*domain.Entitlement, error) {
	var ent *domain.Entitlement
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		p, err := scanPromo(tx.QueryRowContext(ctx, `SELECT `+promoCols+` FROM promo_codes WHERE code = ? FOR UPDATE`, code))
		if err == domain.ErrNoRows {
			return domain.ErrPromoInvalid
		}
		if err != nil {
			return err
		}
		if !p.Active || (p.ExpiresAt != nil && !now.Before(*p.ExpiresAt)) {
			return domain.ErrPromoInvalid
		}
		if p.Used >= p.MaxUses {
			return domain.ErrPromoUsedUp
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO promo_redemptions (code_id, user_id, redeemed_at) VALUES (?, ?, ?)`,
			p.ID, userID, now.Unix()); err != nil {
			if isUnique(err) {
				return domain.ErrPromoRedeemed
			}
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE promo_codes SET used = used + 1 WHERE id = ?`, p.ID); err != nil {
			return err
		}
		e := &domain.Entitlement{
			UserID: userID, Source: domain.SourceCode, Plan: p.Plan, StartsAt: now,
			Note: p.Note, PromoCodeID: &p.ID, PromoCode: p.Code,
		}
		if p.DurationDays != nil {
			end := now.AddDate(0, 0, *p.DurationDays)
			e.EndsAt = &end
		}
		if err := insertEnt(ctx, tx, e); err != nil {
			return err
		}
		ent = e
		return nil
	})
	return ent, err
}
