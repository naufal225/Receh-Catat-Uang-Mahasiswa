package mysql

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"receh/internal/domain"
)

// BillingRepo stores payments, subscriptions and handled gateway events.
//
// Lock order is always organization row, then payment row, then subscription
// row, so concurrent checkouts, webhooks and syncs for one workspace queue up
// instead of deadlocking.
type BillingRepo struct{ db *sql.DB }

func NewBillingRepo(db *sql.DB) *BillingRepo { return &BillingRepo{db} }

const paySelect = `SELECT id, COALESCE(org_id, 0), org_name, payer_id, payer_email, price_id, amount, currency, provider,
	provider_ref, external_id, status, checkout_url, period_start, period_end, expires_at, paid_at, created_at FROM payments`

func scanPayment(row interface{ Scan(...any) error }) (*domain.Payment, error) {
	var p domain.Payment
	var payer, pStart, pEnd, paid sql.NullInt64
	var expires, created int64
	if err := row.Scan(&p.ID, &p.OrgID, &p.OrgName, &payer, &p.PayerEmail, &p.PriceID, &p.Amount, &p.Currency, &p.Provider,
		&p.ProviderRef, &p.ExternalID, &p.Status, &p.CheckoutURL, &pStart, &pEnd, &expires, &paid, &created); err != nil {
		return nil, mapNoRows(err)
	}
	if payer.Valid {
		p.PayerID = &payer.Int64
	}
	p.ExpiresAt, p.CreatedAt = fromUnix(expires), fromUnix(created)
	for _, f := range []struct {
		v   sql.NullInt64
		dst **time.Time
	}{{pStart, &p.PeriodStart}, {pEnd, &p.PeriodEnd}, {paid, &p.PaidAt}} {
		if f.v.Valid {
			t := fromUnix(f.v.Int64)
			*f.dst = &t
		}
	}
	return &p, nil
}

func (r *BillingRepo) Reserve(ctx context.Context, p *domain.Payment, now time.Time) (*domain.Payment, bool, error) {
	var cur *domain.Payment
	created := false
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		cur, created = nil, false
		var lock int64
		if err := tx.QueryRowContext(ctx, `SELECT id FROM organizations WHERE id = ? FOR UPDATE`, p.OrgID).Scan(&lock); err != nil {
			return mapNoRows(err)
		}
		existing, err := scanPayment(tx.QueryRowContext(ctx,
			paySelect+` WHERE org_id = ? AND status = 'pending' AND expires_at > ? ORDER BY id DESC LIMIT 1`, p.OrgID, now.Unix()))
		if err == nil {
			cur = existing
			return nil
		}
		if !errors.Is(err, domain.ErrNoRows) {
			return err
		}
		p.CreatedAt = now.UTC()
		res, err := tx.ExecContext(ctx, `INSERT INTO payments
			(org_id, org_name, payer_id, payer_email, price_id, amount, currency, provider, external_id, status, expires_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
			p.OrgID, p.OrgName, p.PayerID, p.PayerEmail, p.PriceID, p.Amount, p.Currency, p.Provider, p.ExternalID,
			unix(p.ExpiresAt), unix(p.CreatedAt))
		if err != nil {
			return err
		}
		if p.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		p.Status = domain.PaymentPending
		cur, created = p, true
		return nil
	})
	return cur, created, err
}

func (r *BillingRepo) SetCheckout(ctx context.Context, id int64, providerRef, url string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE payments SET provider_ref = ?, checkout_url = ? WHERE id = ?`, providerRef, url, id)
	return err
}

func (r *BillingRepo) Fail(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `UPDATE payments SET status = 'failed' WHERE id = ? AND status = 'pending'`, id)
	return err
}

// Settle applies what the gateway says about one payment, atomically. See
// domain.SettleInput. A paid payment extends the workspace's subscription from
// the later of "now" and the end of the current period, so paying early never
// loses the days left, and adds an entitlement that lasts through the grace
// period; the free plan returns by itself when that entitlement lapses.
func (r *BillingRepo) Settle(ctx context.Context, in domain.SettleInput) (*domain.SettleResult, error) {
	var res *domain.SettleResult
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		res = &domain.SettleResult{}
		var org sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT org_id FROM payments WHERE id = ?`, in.PaymentID).Scan(&org); err != nil {
			return mapNoRows(err)
		}
		var owner int64
		orgAlive := false
		if org.Valid {
			err := tx.QueryRowContext(ctx, `SELECT owner_id FROM organizations WHERE id = ? FOR UPDATE`, org.Int64).Scan(&owner)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return err
			}
			orgAlive = err == nil
		}
		if in.EventID != "" {
			_, err := tx.ExecContext(ctx, `INSERT INTO webhook_events (provider, event_id, received_at) VALUES (?, ?, ?)`,
				in.Provider, in.EventID, in.Now.Unix())
			if isUnique(err) {
				res.Duplicate = true
				return nil
			}
			if err != nil {
				return err
			}
		}
		p, err := scanPayment(tx.QueryRowContext(ctx, paySelect+` WHERE id = ? FOR UPDATE`, in.PaymentID))
		if err != nil {
			return err
		}
		res.Payment = p
		switch in.Status {
		case domain.PaymentPaid:
			if p.Status == domain.PaymentPaid {
				return nil
			}
			start := in.PaidAt
			var sub *domain.Subscription
			if orgAlive {
				if sub, err = scanSub(tx.QueryRowContext(ctx, subSelect+` WHERE org_id = ? FOR UPDATE`, org.Int64)); err != nil && !errors.Is(err, domain.ErrNoRows) {
					return err
				}
				if sub != nil && sub.PeriodEnd.After(start) {
					start = sub.PeriodEnd
				}
			}
			end := in.Price.Extend(start)
			if _, err := tx.ExecContext(ctx, `UPDATE payments SET status = 'paid', paid_at = ?, period_start = ?, period_end = ? WHERE id = ?`,
				in.PaidAt.Unix(), start.Unix(), end.Unix(), p.ID); err != nil {
				return err
			}
			p.Status, p.PaidAt, p.PeriodStart, p.PeriodEnd = domain.PaymentPaid, &in.PaidAt, &start, &end
			res.Activated = true
			if !orgAlive {
				return nil // The workspace was deleted while the payer paid; the money is on record.
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO subscriptions (org_id, price_id, period_end, cancel_at_period_end, state, last_payment_id, updated_at)
				VALUES (?, ?, ?, 0, 'active', ?, ?)
				ON DUPLICATE KEY UPDATE price_id = VALUES(price_id), period_end = VALUES(period_end), cancel_at_period_end = 0,
					state = 'active', last_payment_id = VALUES(last_payment_id), updated_at = VALUES(updated_at)`,
				org.Int64, p.PriceID, end.Unix(), p.ID, in.Now.Unix()); err != nil {
				return err
			}
			until := end.Add(in.Grace)
			ent := &domain.Entitlement{UserID: owner, OrgID: &org.Int64, Source: domain.SourcePaid, Plan: domain.PlanPremium,
				StartsAt: in.PaidAt, EndsAt: &until, Note: in.Note}
			if err := insertEnt(ctx, tx, ent); err != nil {
				return err
			}
			res.Subscription = &domain.Subscription{OrgID: org.Int64, PriceID: p.PriceID, PeriodEnd: end, Stored: domain.SubActive}
		case domain.PaymentExpired, domain.PaymentFailed:
			if p.Status != domain.PaymentPending {
				return nil
			}
			if _, err := tx.ExecContext(ctx, `UPDATE payments SET status = ? WHERE id = ?`, string(in.Status), p.ID); err != nil {
				return err
			}
			p.Status = in.Status
		}
		return nil
	})
	return res, err
}

func (r *BillingRepo) PaymentByID(ctx context.Context, id int64) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRowContext(ctx, paySelect+` WHERE id = ?`, id))
}

func (r *BillingRepo) PaymentByExternalID(ctx context.Context, externalID string) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRowContext(ctx, paySelect+` WHERE external_id = ?`, externalID))
}

func (r *BillingRepo) PendingFor(ctx context.Context, orgID int64, now time.Time) (*domain.Payment, error) {
	return scanPayment(r.db.QueryRowContext(ctx,
		paySelect+` WHERE org_id = ? AND status = 'pending' AND expires_at > ? ORDER BY id DESC LIMIT 1`, orgID, now.Unix()))
}

func (r *BillingRepo) queryPayments(ctx context.Context, q string, args ...any) ([]domain.Payment, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.Payment{}
	for rows.Next() {
		p, err := scanPayment(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *BillingRepo) PaymentsForOrg(ctx context.Context, orgID int64, limit int) ([]domain.Payment, error) {
	return r.queryPayments(ctx, paySelect+` WHERE org_id = ? ORDER BY id DESC LIMIT ?`, orgID, limit)
}

func (r *BillingRepo) PaymentsAll(ctx context.Context, status domain.PaymentStatus, limit, offset int) ([]domain.Payment, int, error) {
	where, args := ``, []any{}
	if status != "" {
		where, args = ` WHERE status = ?`, append(args, string(status))
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM payments`+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	list, err := r.queryPayments(ctx, paySelect+where+` ORDER BY id DESC LIMIT ? OFFSET ?`, append(args, limit, offset)...)
	return list, total, err
}

func (r *BillingRepo) PendingBefore(ctx context.Context, t time.Time, limit int) ([]domain.Payment, error) {
	return r.queryPayments(ctx, paySelect+` WHERE status = 'pending' AND created_at <= ? ORDER BY id LIMIT ?`, t.Unix(), limit)
}

// ---- Subscriptions ------------------------------------------------------------------

const subSelect = `SELECT org_id, price_id, period_end, cancel_at_period_end, state, last_payment_id, updated_at FROM subscriptions`

func scanSub(row interface{ Scan(...any) error }) (*domain.Subscription, error) {
	var s domain.Subscription
	var end, updated int64
	var cancel int
	var last sql.NullInt64
	if err := row.Scan(&s.OrgID, &s.PriceID, &end, &cancel, &s.Stored, &last, &updated); err != nil {
		return nil, mapNoRows(err)
	}
	s.PeriodEnd, s.UpdatedAt, s.CancelAtPeriodEnd = fromUnix(end), fromUnix(updated), cancel == 1
	if last.Valid {
		s.LastPaymentID = &last.Int64
	}
	return &s, nil
}

func (r *BillingRepo) SubscriptionOf(ctx context.Context, orgID int64) (*domain.Subscription, error) {
	return scanSub(r.db.QueryRowContext(ctx, subSelect+` WHERE org_id = ?`, orgID))
}

func (r *BillingRepo) OwnedSubscriptions(ctx context.Context, userID int64) ([]domain.OwnedSubscription, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT s.org_id, s.price_id, s.period_end, s.cancel_at_period_end, s.state, s.last_payment_id,
		s.updated_at, o.name, o.personal FROM subscriptions s JOIN organizations o ON o.id = s.org_id WHERE o.owner_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.OwnedSubscription{}
	for rows.Next() {
		var s domain.OwnedSubscription
		var end, updated int64
		var cancel, personal int
		var last sql.NullInt64
		if err := rows.Scan(&s.OrgID, &s.PriceID, &end, &cancel, &s.Stored, &last, &updated, &s.OrgName, &personal); err != nil {
			return nil, err
		}
		s.Personal = personal == 1
		s.PeriodEnd, s.UpdatedAt, s.CancelAtPeriodEnd = fromUnix(end), fromUnix(updated), cancel == 1
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *BillingRepo) SetCancelAtPeriodEnd(ctx context.Context, orgID int64, cancel bool) (bool, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE subscriptions SET cancel_at_period_end = ?, updated_at = ? WHERE org_id = ?`,
		boolInt(cancel), time.Now().Unix(), orgID)
	if err != nil {
		return false, err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		return true, nil
	}
	var exists int
	err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM subscriptions WHERE org_id = ?`, orgID).Scan(&exists)
	return exists > 0, err
}

func (r *BillingRepo) SyncSubscriptionStates(ctx context.Context, now time.Time, grace time.Duration) error {
	g := int64(grace / time.Second)
	if _, err := r.db.ExecContext(ctx, `UPDATE subscriptions SET state = 'expired', updated_at = ?
		WHERE state <> 'expired' AND period_end + ? <= ?`, now.Unix(), g, now.Unix()); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE subscriptions SET state = 'grace', updated_at = ?
		WHERE state = 'active' AND period_end <= ? AND period_end + ? > ?`, now.Unix(), now.Unix(), g, now.Unix())
	return err
}
