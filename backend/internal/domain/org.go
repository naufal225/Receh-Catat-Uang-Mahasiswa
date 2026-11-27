package domain

import (
	"context"
	"time"
)

// OrgRole is what a member may do in an organization (workspace).
//
//	owner  - exactly one per org: billing, delete the org, transfer ownership
//	admin  - manage members and invitations
//	member - use the org's data (modules decide the rest)
type OrgRole string

const (
	OrgOwner  OrgRole = "owner"
	OrgAdmin  OrgRole = "admin"
	OrgMember OrgRole = "member"
)

func (r OrgRole) Valid() bool { return r == OrgOwner || r == OrgAdmin || r == OrgMember }

// CanManage reports whether the role may manage members and invitations.
func (r OrgRole) CanManage() bool { return r == OrgOwner || r == OrgAdmin }

// Org is a workspace: the unit that owns module data and (from Phase C) a
// subscription. Every user account has a personal org; teams are extra orgs.
type Org struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Personal  bool      `json:"personal"`
	OwnerID   int64     `json:"ownerId"`
	CreatedAt time.Time `json:"createdAt"`
}

// OrgMembership is an org as seen by one of its members.
type OrgMembership struct {
	Org
	Role OrgRole `json:"role"`
}

// OrgPerson is one person in an org.
type OrgPerson struct {
	UserID   int64     `json:"userId"`
	Name     string    `json:"name"`
	Email    string    `json:"email"`
	Role     OrgRole   `json:"role"`
	JoinedAt time.Time `json:"joinedAt"`
}

// OrgInvite is an invitation by email. Only a hash of the token is stored; the
// plain token exists once, in the link handed to the inviter.
type OrgInvite struct {
	ID          int64      `json:"id"`
	OrgID       int64      `json:"orgId"`
	OrgName     string     `json:"orgName,omitempty"`
	Email       string     `json:"email"`
	Role        OrgRole    `json:"role"`
	InvitedBy   string     `json:"invitedBy"`
	InvitedByID int64      `json:"-"`
	ExpiresAt   time.Time  `json:"expiresAt"`
	AcceptedAt  *time.Time `json:"acceptedAt,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
}

type OrgRepository interface {
	// Create inserts the org and its owner membership in one commit.
	Create(ctx context.Context, o *Org) error
	// Delete removes an org and (by cascade) everything it owns.
	Delete(ctx context.Context, id int64) error
	ByID(ctx context.Context, id int64) (*Org, error)
	Rename(ctx context.Context, id int64, name string) error
	ListForUser(ctx context.Context, userID int64) ([]OrgMembership, error)
	// RoleOf returns the user's role in the org, or ErrNoRows when not a member.
	RoleOf(ctx context.Context, orgID, userID int64) (OrgRole, error)
	Members(ctx context.Context, orgID int64) ([]OrgPerson, error)
	MemberIDs(ctx context.Context, orgID int64) ([]int64, error)
	AddMember(ctx context.Context, orgID, userID int64, role OrgRole) error
	SetRole(ctx context.Context, orgID, userID int64, role OrgRole) error
	RemoveMember(ctx context.Context, orgID, userID int64) error
	// TransferOwner makes toUser the owner and the previous owner an admin.
	TransferOwner(ctx context.Context, orgID, toUser int64) error

	CreateInvite(ctx context.Context, i *OrgInvite, tokenHash string) error
	PendingInvites(ctx context.Context, orgID int64, now time.Time) ([]OrgInvite, error)
	InviteByToken(ctx context.Context, tokenHash string) (*OrgInvite, error)
	RevokeInvite(ctx context.Context, orgID, inviteID int64) (bool, error)
	// AcceptInvite marks the invitation used and adds the member in one
	// commit. It fails with ErrNoRows when the invitation was already used.
	AcceptInvite(ctx context.Context, tokenHash string, userID int64, now time.Time) (*OrgInvite, error)
	CountPending(ctx context.Context, orgID int64, now time.Time) (int, error)
}

// OrgContext is the workspace a request acts in, already checked against the
// caller's membership.
type OrgContext struct {
	ID       int64   `json:"id"`
	Role     OrgRole `json:"role"`
	Personal bool    `json:"personal"`
}
