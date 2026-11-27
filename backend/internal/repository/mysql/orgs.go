package mysql

import (
	"context"
	"database/sql"
	"time"

	"receh/internal/domain"
)

type OrgRepo struct{ db *sql.DB }

func NewOrgRepo(db *sql.DB) *OrgRepo { return &OrgRepo{db} }

func (r *OrgRepo) Create(ctx context.Context, o *domain.Org) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO organizations (name, personal, owner_id, created_at) VALUES (?, ?, ?, ?)`,
			o.Name, boolInt(o.Personal), o.OwnerID, unix(now))
		if err != nil {
			return err
		}
		if o.ID, err = res.LastInsertId(); err != nil {
			return err
		}
		o.CreatedAt = now
		_, err = tx.ExecContext(ctx,
			`INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?, ?, 'owner', ?)`,
			o.ID, o.OwnerID, unix(now))
		return err
	})
}

func (r *OrgRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM organizations WHERE id = ?`, id)
	return err
}

const orgCols = `id, name, personal, owner_id, created_at`

func scanOrg(row interface{ Scan(...any) error }) (*domain.Org, error) {
	var o domain.Org
	var personal int
	var created int64
	if err := row.Scan(&o.ID, &o.Name, &personal, &o.OwnerID, &created); err != nil {
		return nil, mapNoRows(err)
	}
	o.Personal, o.CreatedAt = personal == 1, fromUnix(created)
	return &o, nil
}

func (r *OrgRepo) ByID(ctx context.Context, id int64) (*domain.Org, error) {
	return scanOrg(r.db.QueryRowContext(ctx, `SELECT `+orgCols+` FROM organizations WHERE id = ?`, id))
}

func (r *OrgRepo) Rename(ctx context.Context, id int64, name string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE organizations SET name = ? WHERE id = ?`, name, id)
	return err
}

func (r *OrgRepo) ListForUser(ctx context.Context, userID int64) ([]domain.OrgMembership, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT o.id, o.name, o.personal, o.owner_id, o.created_at, m.role
		FROM org_members m JOIN organizations o ON o.id = m.org_id
		WHERE m.user_id = ?
		ORDER BY o.personal DESC, o.id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.OrgMembership{}
	for rows.Next() {
		var m domain.OrgMembership
		var personal int
		var created int64
		if err := rows.Scan(&m.ID, &m.Name, &personal, &m.OwnerID, &created, &m.Role); err != nil {
			return nil, err
		}
		m.Personal, m.CreatedAt = personal == 1, fromUnix(created)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *OrgRepo) RoleOf(ctx context.Context, orgID, userID int64) (domain.OrgRole, error) {
	var role domain.OrgRole
	err := r.db.QueryRowContext(ctx,
		`SELECT role FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, userID).Scan(&role)
	return role, mapNoRows(err)
}

func (r *OrgRepo) Members(ctx context.Context, orgID int64) ([]domain.OrgPerson, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT u.id, u.name, u.email, m.role, m.created_at
		FROM org_members m JOIN users u ON u.id = m.user_id
		WHERE m.org_id = ?
		ORDER BY FIELD(m.role, 'owner', 'admin', 'member'), u.name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.OrgPerson{}
	for rows.Next() {
		var m domain.OrgPerson
		var joined int64
		if err := rows.Scan(&m.UserID, &m.Name, &m.Email, &m.Role, &joined); err != nil {
			return nil, err
		}
		m.JoinedAt = fromUnix(joined)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (r *OrgRepo) MemberIDs(ctx context.Context, orgID int64) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT user_id FROM org_members WHERE org_id = ?`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *OrgRepo) AddMember(ctx context.Context, orgID, userID int64, role domain.OrgRole) error {
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO org_members (org_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
		orgID, userID, role, unix(time.Now().UTC()))
	if isUnique(err) {
		return domain.Conflict("ALREADY_MEMBER", "Orang ini sudah menjadi anggota.")
	}
	return err
}

func (r *OrgRepo) SetRole(ctx context.Context, orgID, userID int64, role domain.OrgRole) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE org_members SET role = ? WHERE org_id = ? AND user_id = ?`, role, orgID, userID)
	return err
}

func (r *OrgRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM org_members WHERE org_id = ? AND user_id = ?`, orgID, userID)
	return err
}

func (r *OrgRepo) TransferOwner(ctx context.Context, orgID, toUser int64) error {
	return inTx(ctx, r.db, func(tx *sql.Tx) error {
		var from int64
		if err := tx.QueryRowContext(ctx, `SELECT owner_id FROM organizations WHERE id = ? FOR UPDATE`, orgID).Scan(&from); err != nil {
			return mapNoRows(err)
		}
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM org_members WHERE org_id = ? AND user_id = ? FOR UPDATE`, orgID, toUser).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return domain.ErrNoRows
		}
		if _, err := tx.ExecContext(ctx, `UPDATE org_members SET role = 'admin' WHERE org_id = ? AND user_id = ?`, orgID, from); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE org_members SET role = 'owner' WHERE org_id = ? AND user_id = ?`, orgID, toUser); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE organizations SET owner_id = ? WHERE id = ?`, toUser, orgID)
		return err
	})
}

// ---- Invitations ---------------------------------------------------------------

func (r *OrgRepo) CreateInvite(ctx context.Context, i *domain.OrgInvite, tokenHash string) error {
	now := time.Now().UTC()
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO org_invitations (org_id, email, role, token_hash, invited_by, expires_at, created_at)
		VALUES (?, ?, ?, ?, NULLIF(?, 0), ?, ?)`,
		i.OrgID, i.Email, i.Role, tokenHash, i.InvitedByID, unix(i.ExpiresAt), unix(now))
	if err != nil {
		return err
	}
	i.ID, err = res.LastInsertId()
	i.CreatedAt = now
	return err
}

const inviteSelect = `
	SELECT i.id, i.org_id, o.name, i.email, i.role, COALESCE(u.name, ''), i.expires_at, i.accepted_at, i.created_at
	FROM org_invitations i
	JOIN organizations o ON o.id = i.org_id
	LEFT JOIN users u ON u.id = i.invited_by`

func scanInvite(row interface{ Scan(...any) error }) (*domain.OrgInvite, error) {
	var i domain.OrgInvite
	var expires, created int64
	var accepted sql.NullInt64
	if err := row.Scan(&i.ID, &i.OrgID, &i.OrgName, &i.Email, &i.Role, &i.InvitedBy, &expires, &accepted, &created); err != nil {
		return nil, mapNoRows(err)
	}
	i.ExpiresAt, i.CreatedAt = fromUnix(expires), fromUnix(created)
	if accepted.Valid {
		t := fromUnix(accepted.Int64)
		i.AcceptedAt = &t
	}
	return &i, nil
}

func (r *OrgRepo) PendingInvites(ctx context.Context, orgID int64, now time.Time) ([]domain.OrgInvite, error) {
	rows, err := r.db.QueryContext(ctx,
		inviteSelect+` WHERE i.org_id = ? AND i.accepted_at IS NULL AND i.expires_at > ? ORDER BY i.id DESC`, orgID, unix(now))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.OrgInvite{}
	for rows.Next() {
		i, err := scanInvite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *i)
	}
	return out, rows.Err()
}

func (r *OrgRepo) InviteByToken(ctx context.Context, tokenHash string) (*domain.OrgInvite, error) {
	return scanInvite(r.db.QueryRowContext(ctx, inviteSelect+` WHERE i.token_hash = ?`, tokenHash))
}

func (r *OrgRepo) RevokeInvite(ctx context.Context, orgID, inviteID int64) (bool, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM org_invitations WHERE id = ? AND org_id = ? AND accepted_at IS NULL`, inviteID, orgID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func (r *OrgRepo) AcceptInvite(ctx context.Context, tokenHash string, userID int64, now time.Time) (*domain.OrgInvite, error) {
	var out *domain.OrgInvite
	err := inTx(ctx, r.db, func(tx *sql.Tx) error {
		var id, orgID int64
		var role domain.OrgRole
		var expires int64
		var accepted sql.NullInt64
		// FOR UPDATE: two people opening the same link serialize here, and the
		// second sees accepted_at already set.
		err := tx.QueryRowContext(ctx,
			`SELECT id, org_id, role, expires_at, accepted_at FROM org_invitations WHERE token_hash = ? FOR UPDATE`, tokenHash).
			Scan(&id, &orgID, &role, &expires, &accepted)
		if err != nil {
			return mapNoRows(err)
		}
		if accepted.Valid || expires <= unix(now) {
			return domain.ErrNoRows
		}
		if _, err := tx.ExecContext(ctx, `UPDATE org_invitations SET accepted_at = ? WHERE id = ?`, unix(now), id); err != nil {
			return err
		}
		// An existing member keeps their current role; the link is still used up.
		if _, err := tx.ExecContext(ctx,
			`INSERT IGNORE INTO org_members (org_id, user_id, role, created_at) VALUES (?, ?, ?, ?)`,
			orgID, userID, role, unix(now)); err != nil {
			return err
		}
		out = &domain.OrgInvite{ID: id, OrgID: orgID, Role: role}
		return nil
	})
	return out, err
}

func (r *OrgRepo) CountPending(ctx context.Context, orgID int64, now time.Time) (int, error) {
	var n int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM org_invitations WHERE org_id = ? AND accepted_at IS NULL AND expires_at > ?`, orgID, unix(now)).Scan(&n)
	return n, err
}
