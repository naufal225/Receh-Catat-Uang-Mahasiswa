package usecase

import (
	"context"
	"errors"
	"strings"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/cache"
	"receh/internal/platform/ratelimit"
)

const (
	inviteTTL        = 7 * 24 * time.Hour
	maxPendingInvite = 50
	maxOwnedOrgs     = 10
	roleCacheTTL     = 30 * time.Second
)

type roleKey struct{ org, user int64 }

// OrgService owns workspaces: creation, membership, roles and invitations.
// Every rule about who may do what in an org lives here, so modules only ever
// ask "which org, and what is my role".
type OrgService struct {
	orgs  domain.OrgRepository
	users domain.UserRepository
	audit *AuditRecorder
	clock domain.Clock

	hooks []func(ctx context.Context, orgID int64) error

	// Membership is read on every request, so roles are cached briefly. Every
	// change made here drops the affected entries at once.
	roles  *cache.TTL[roleKey, domain.OrgRole]
	owners *cache.TTL[int64, int64]

	inviteLimit *ratelimit.Limiter
}

func NewOrgService(o domain.OrgRepository, u domain.UserRepository, a *AuditRecorder, clock domain.Clock, cacheTTL time.Duration) *OrgService {
	return &OrgService{
		orgs: o, users: u, audit: a, clock: clock,
		roles:       cache.New[roleKey, domain.OrgRole](cacheTTL, 20_000),
		owners:      cache.New[int64, int64](cacheTTL, 5_000),
		inviteLimit: ratelimit.New(30, time.Hour),
	}
}

// RunJanitor sweeps expired cache entries until ctx ends.
func (s *OrgService) RunJanitor(ctx context.Context) {
	go s.owners.RunJanitor(ctx, time.Minute)
	s.roles.RunJanitor(ctx, time.Minute)
}

// OnOrgCreated registers a hook that prepares a new workspace (for example
// starter categories). Creation is all-or-nothing: if a hook fails the
// workspace is removed again. Register hooks before serving.
func (s *OrgService) OnOrgCreated(fn func(ctx context.Context, orgID int64) error) {
	s.hooks = append(s.hooks, fn)
}

func (s *OrgService) create(ctx context.Context, o *domain.Org) error {
	if err := s.orgs.Create(ctx, o); err != nil {
		return err
	}
	for _, fn := range s.hooks {
		if err := fn(ctx, o.ID); err != nil {
			_ = s.orgs.Delete(context.WithoutCancel(ctx), o.ID)
			return err
		}
	}
	return nil
}

// CreatePersonal makes the account's own workspace. Sign-up calls it.
func (s *OrgService) CreatePersonal(ctx context.Context, u *domain.User) error {
	return s.create(ctx, &domain.Org{Name: "Pribadi", Personal: true, OwnerID: u.ID})
}

// Create makes a team workspace owned by the caller.
func (s *OrgService) Create(ctx context.Context, u *domain.User, name string, client domain.ClientInfo) (*domain.OrgMembership, error) {
	name, err := cleanName("name", name, 60, "Nama tim wajib diisi.")
	if err != nil {
		return nil, err
	}
	mine, err := s.orgs.ListForUser(ctx, u.ID)
	if err != nil {
		return nil, err
	}
	owned := 0
	for _, m := range mine {
		if m.Role == domain.OrgOwner {
			owned++
		}
	}
	if owned >= maxOwnedOrgs {
		return nil, domain.Conflict("ORG_LIMIT", "Kamu sudah memiliki "+itoa(maxOwnedOrgs)+" workspace.")
	}
	o := &domain.Org{Name: name, OwnerID: u.ID}
	if err := s.create(ctx, o); err != nil {
		return nil, err
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventOrgCreated, Client: client, Detail: "Membuat workspace " + name})
	return &domain.OrgMembership{Org: *o, Role: domain.OrgOwner}, nil
}

func (s *OrgService) List(ctx context.Context, userID int64) ([]domain.OrgMembership, error) {
	return s.orgs.ListForUser(ctx, userID)
}

// Resolve checks that userID belongs to orgID and returns the context a request
// acts in. orgID 0 means "the account's personal workspace".
func (s *OrgService) Resolve(ctx context.Context, userID, orgID int64) (domain.OrgContext, error) {
	if orgID == 0 {
		mine, err := s.orgs.ListForUser(ctx, userID)
		if err != nil {
			return domain.OrgContext{}, err
		}
		for _, m := range mine {
			if m.Personal {
				return domain.OrgContext{ID: m.ID, Role: m.Role, Personal: true}, nil
			}
		}
		return domain.OrgContext{}, domain.Forbidden("NO_WORKSPACE", "Akun ini belum punya workspace.")
	}
	role, err := s.roleOf(ctx, orgID, userID)
	if errors.Is(err, domain.ErrNoRows) {
		return domain.OrgContext{}, domain.Forbidden("NOT_A_MEMBER", "Kamu bukan anggota workspace ini.")
	}
	if err != nil {
		return domain.OrgContext{}, err
	}
	o, err := s.orgs.ByID(ctx, orgID)
	if err != nil {
		return domain.OrgContext{}, notFoundOr(err, "Workspace tidak ditemukan.")
	}
	return domain.OrgContext{ID: orgID, Role: role, Personal: o.Personal}, nil
}

func (s *OrgService) roleOf(ctx context.Context, orgID, userID int64) (domain.OrgRole, error) {
	k := roleKey{orgID, userID}
	if r, ok := s.roles.Get(k); ok {
		return r, nil
	}
	r, err := s.orgs.RoleOf(ctx, orgID, userID)
	if err == nil {
		s.roles.Set(k, r)
	}
	return r, err
}

// OwnerOf returns who owns (and pays for) an org.
func (s *OrgService) OwnerOf(ctx context.Context, orgID int64) (int64, error) {
	if id, ok := s.owners.Get(orgID); ok {
		return id, nil
	}
	o, err := s.orgs.ByID(ctx, orgID)
	if err != nil {
		return 0, notFoundOr(err, "Workspace tidak ditemukan.")
	}
	s.owners.Set(orgID, o.OwnerID)
	return o.OwnerID, nil
}

// MemberIDs lists the users of an org (for notification fan-out).
func (s *OrgService) MemberIDs(ctx context.Context, orgID int64) ([]int64, error) {
	return s.orgs.MemberIDs(ctx, orgID)
}

func (s *OrgService) forget(orgID int64, users ...int64) {
	for _, u := range users {
		s.roles.Delete(roleKey{orgID, u})
	}
	s.owners.Delete(orgID)
}

func (s *OrgService) forgetOrg(orgID int64) {
	s.roles.DeleteFunc(func(k roleKey, _ domain.OrgRole) bool { return k.org == orgID })
	s.owners.Delete(orgID)
}

func (s *OrgService) record(actor *domain.User, client domain.ClientInfo, ev domain.AuditEvent, detail string) {
	s.audit.Record(AuditEntry{UserID: &actor.ID, Email: actor.Email, Event: ev, Client: client, Detail: detail})
}

// require loads the org and checks the actor's role in it.
func (s *OrgService) require(ctx context.Context, actor *domain.User, orgID int64, manage bool) (*domain.Org, domain.OrgRole, error) {
	role, err := s.roleOf(ctx, orgID, actor.ID)
	if errors.Is(err, domain.ErrNoRows) {
		return nil, "", domain.Forbidden("NOT_A_MEMBER", "Kamu bukan anggota workspace ini.")
	}
	if err != nil {
		return nil, "", err
	}
	if manage && !role.CanManage() {
		return nil, "", domain.Forbidden("ORG_FORBIDDEN", "Hanya owner atau admin yang bisa melakukan ini.")
	}
	o, err := s.orgs.ByID(ctx, orgID)
	if err != nil {
		return nil, "", notFoundOr(err, "Workspace tidak ditemukan.")
	}
	return o, role, nil
}

func (s *OrgService) Rename(ctx context.Context, actor *domain.User, orgID int64, name string, client domain.ClientInfo) error {
	name, err := cleanName("name", name, 60, "Nama workspace wajib diisi.")
	if err != nil {
		return err
	}
	o, _, err := s.require(ctx, actor, orgID, true)
	if err != nil {
		return err
	}
	if o.Personal {
		return domain.Conflict("PERSONAL_ORG", "Workspace pribadi tidak bisa diganti namanya.")
	}
	if err := s.orgs.Rename(ctx, orgID, name); err != nil {
		return err
	}
	s.record(actor, client, domain.EventOrgChanged, "Mengganti nama workspace "+o.Name+" menjadi "+name)
	return nil
}

// Delete removes a team workspace and everything in it. Only its owner may.
func (s *OrgService) Delete(ctx context.Context, actor *domain.User, orgID int64, client domain.ClientInfo) error {
	o, role, err := s.require(ctx, actor, orgID, false)
	if err != nil {
		return err
	}
	if role != domain.OrgOwner {
		return domain.Forbidden("ORG_FORBIDDEN", "Hanya owner yang bisa menghapus workspace.")
	}
	if o.Personal {
		return domain.Conflict("PERSONAL_ORG", "Workspace pribadi tidak bisa dihapus.")
	}
	if err := s.orgs.Delete(ctx, orgID); err != nil {
		return err
	}
	s.forgetOrg(orgID)
	s.record(actor, client, domain.EventOrgChanged, "Menghapus workspace "+o.Name)
	return nil
}

func (s *OrgService) Members(ctx context.Context, actor *domain.User, orgID int64) ([]domain.OrgPerson, error) {
	if _, _, err := s.require(ctx, actor, orgID, false); err != nil {
		return nil, err
	}
	return s.orgs.Members(ctx, orgID)
}

// SetRole changes a member between admin and member. Only the owner does it;
// ownership moves through TransferOwner.
func (s *OrgService) SetRole(ctx context.Context, actor *domain.User, orgID, target int64, role domain.OrgRole, client domain.ClientInfo) error {
	if role != domain.OrgAdmin && role != domain.OrgMember {
		return domain.Invalid("role", "Peran harus admin atau member. Untuk memindahkan kepemilikan, pakai pindahkan owner.")
	}
	o, mine, err := s.require(ctx, actor, orgID, false)
	if err != nil {
		return err
	}
	if mine != domain.OrgOwner {
		return domain.Forbidden("ORG_FORBIDDEN", "Hanya owner yang bisa mengubah peran.")
	}
	if o.Personal {
		return domain.Conflict("PERSONAL_ORG", "Workspace pribadi hanya untuk pemiliknya.")
	}
	cur, err := s.orgs.RoleOf(ctx, orgID, target)
	if err != nil {
		return notFoundOr(err, "Anggota tidak ditemukan.")
	}
	if cur == domain.OrgOwner {
		return domain.Conflict("OWNER_PROTECTED", "Peran owner tidak bisa diubah. Pindahkan kepemilikan dulu.")
	}
	if err := s.orgs.SetRole(ctx, orgID, target, role); err != nil {
		return err
	}
	s.forget(orgID, target)
	s.record(actor, client, domain.EventOrgMember, "Peran anggota "+itoa64(target)+" di "+o.Name+": "+string(cur)+" → "+string(role))
	return nil
}

// RemoveMember removes someone, or lets a member leave (target == actor).
// The owner cannot leave or be removed; admins can only remove plain members.
func (s *OrgService) RemoveMember(ctx context.Context, actor *domain.User, orgID, target int64, client domain.ClientInfo) error {
	o, mine, err := s.require(ctx, actor, orgID, false)
	if err != nil {
		return err
	}
	if o.Personal {
		return domain.Conflict("PERSONAL_ORG", "Workspace pribadi hanya untuk pemiliknya.")
	}
	cur, err := s.orgs.RoleOf(ctx, orgID, target)
	if err != nil {
		return notFoundOr(err, "Anggota tidak ditemukan.")
	}
	if cur == domain.OrgOwner {
		return domain.Conflict("OWNER_PROTECTED", "Owner tidak bisa keluar atau dikeluarkan. Pindahkan kepemilikan atau hapus workspace.")
	}
	if target != actor.ID {
		switch {
		case mine == domain.OrgOwner:
		case mine == domain.OrgAdmin && cur == domain.OrgMember:
		default:
			return domain.Forbidden("ORG_FORBIDDEN", "Kamu tidak bisa mengeluarkan anggota ini.")
		}
	}
	if err := s.orgs.RemoveMember(ctx, orgID, target); err != nil {
		return err
	}
	s.forget(orgID, target)
	verb := "Mengeluarkan anggota " + itoa64(target) + " dari "
	if target == actor.ID {
		verb = "Keluar dari "
	}
	s.record(actor, client, domain.EventOrgMember, verb+o.Name)
	return nil
}

// TransferOwner hands ownership (and, later, the subscription) to another
// member; the previous owner becomes an admin.
func (s *OrgService) TransferOwner(ctx context.Context, actor *domain.User, orgID, to int64, client domain.ClientInfo) error {
	o, mine, err := s.require(ctx, actor, orgID, false)
	if err != nil {
		return err
	}
	if mine != domain.OrgOwner {
		return domain.Forbidden("ORG_FORBIDDEN", "Hanya owner yang bisa memindahkan kepemilikan.")
	}
	if o.Personal {
		return domain.Conflict("PERSONAL_ORG", "Workspace pribadi tidak bisa dipindahkan.")
	}
	if to == actor.ID {
		return domain.Invalid("userId", "Pilih anggota lain.")
	}
	if err := s.orgs.TransferOwner(ctx, orgID, to); err != nil {
		return notFoundOr(err, "Anggota tidak ditemukan.")
	}
	s.forget(orgID, actor.ID, to)
	s.record(actor, client, domain.EventOrgMember, "Memindahkan kepemilikan "+o.Name+" ke "+itoa64(to))
	return nil
}

// ---- Invitations ---------------------------------------------------------------

// InviteResult carries the one-time token. Show the link once; only its hash
// is stored.
type InviteResult struct {
	Invite domain.OrgInvite `json:"invite"`
	Token  string           `json:"token"`
}

func (s *OrgService) Invite(ctx context.Context, actor *domain.User, orgID int64, email string, role domain.OrgRole, client domain.ClientInfo) (*InviteResult, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, err
	}
	if role != domain.OrgAdmin && role != domain.OrgMember {
		return nil, domain.Invalid("role", "Peran harus admin atau member.")
	}
	o, mine, err := s.require(ctx, actor, orgID, true)
	if err != nil {
		return nil, err
	}
	if o.Personal {
		return nil, domain.Conflict("PERSONAL_ORG", "Workspace pribadi tidak bisa diundangi anggota. Buat tim dulu.")
	}
	if role == domain.OrgAdmin && mine != domain.OrgOwner {
		return nil, domain.Forbidden("ORG_FORBIDDEN", "Hanya owner yang bisa mengundang admin.")
	}
	key := itoa64(actor.ID)
	if s.inviteLimit.Blocked(key) {
		return nil, domain.TooMany("Terlalu banyak undangan dalam satu jam. Coba lagi nanti.")
	}
	now := s.clock.Now()
	if n, err := s.orgs.CountPending(ctx, orgID, now); err != nil {
		return nil, err
	} else if n >= maxPendingInvite {
		return nil, domain.Conflict("TOO_MANY_INVITES", "Terlalu banyak undangan yang belum diterima. Batalkan sebagian dulu.")
	}
	// Already a member? Say so instead of mailing a useless link.
	if u, err := s.users.ByEmail(ctx, email); err == nil {
		if _, err := s.orgs.RoleOf(ctx, orgID, u.ID); err == nil {
			return nil, domain.Conflict("ALREADY_MEMBER", "Orang ini sudah menjadi anggota.")
		}
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	inv := domain.OrgInvite{OrgID: orgID, OrgName: o.Name, Email: email, Role: role,
		InvitedBy: actor.Name, InvitedByID: actor.ID, ExpiresAt: now.Add(inviteTTL)}
	if err := s.orgs.CreateInvite(ctx, &inv, HashToken(token)); err != nil {
		return nil, err
	}
	s.inviteLimit.Hit(key)
	s.record(actor, client, domain.EventOrgMember, "Mengundang "+email+" ke "+o.Name+" sebagai "+string(role))
	return &InviteResult{Invite: inv, Token: token}, nil
}

func (s *OrgService) PendingInvites(ctx context.Context, actor *domain.User, orgID int64) ([]domain.OrgInvite, error) {
	if _, _, err := s.require(ctx, actor, orgID, true); err != nil {
		return nil, err
	}
	return s.orgs.PendingInvites(ctx, orgID, s.clock.Now())
}

func (s *OrgService) RevokeInvite(ctx context.Context, actor *domain.User, orgID, inviteID int64, client domain.ClientInfo) error {
	o, _, err := s.require(ctx, actor, orgID, true)
	if err != nil {
		return err
	}
	ok, err := s.orgs.RevokeInvite(ctx, orgID, inviteID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NotFound("Undangan tidak ditemukan.")
	}
	s.record(actor, client, domain.EventOrgMember, "Membatalkan undangan ke "+o.Name)
	return nil
}

var errInviteGone = domain.NotFound("Undangan tidak ditemukan, sudah dipakai, atau kedaluwarsa.")

// InviteInfo shows an invitation to the signed-in person it is meant for.
// Anyone else gets the same "not found", so tokens cannot be probed.
func (s *OrgService) InviteInfo(ctx context.Context, u *domain.User, token string) (*domain.OrgInvite, error) {
	inv, err := s.orgs.InviteByToken(ctx, HashToken(strings.TrimSpace(token)))
	if err != nil {
		if errors.Is(err, domain.ErrNoRows) {
			return nil, errInviteGone
		}
		return nil, err
	}
	if inv.AcceptedAt != nil || !inv.ExpiresAt.After(s.clock.Now()) || !strings.EqualFold(inv.Email, u.Email) {
		return nil, errInviteGone
	}
	return inv, nil
}

// Accept joins the invited org. Two people opening one link cannot both
// succeed: the repository serializes on the invitation row.
func (s *OrgService) Accept(ctx context.Context, u *domain.User, token string, client domain.ClientInfo) (*domain.OrgMembership, error) {
	hash := HashToken(strings.TrimSpace(token))
	if _, err := s.InviteInfo(ctx, u, token); err != nil {
		return nil, err
	}
	inv, err := s.orgs.AcceptInvite(ctx, hash, u.ID, s.clock.Now())
	if errors.Is(err, domain.ErrNoRows) {
		return nil, errInviteGone
	}
	if err != nil {
		return nil, err
	}
	s.forget(inv.OrgID, u.ID)
	o, err := s.orgs.ByID(ctx, inv.OrgID)
	if err != nil {
		return nil, err
	}
	role, err := s.orgs.RoleOf(ctx, inv.OrgID, u.ID)
	if err != nil {
		return nil, err
	}
	s.record(u, client, domain.EventOrgMember, "Bergabung ke "+o.Name+" sebagai "+string(role))
	return &domain.OrgMembership{Org: *o, Role: role}, nil
}
