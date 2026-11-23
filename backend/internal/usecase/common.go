// Package usecase contains the core behaviour (accounts, sessions, admin,
// plans, notifications). It depends only on the domain and on ports, never on
// HTTP or SQL. Feature modules live under internal/modules.
package usecase

import (
	"context"
	"regexp"
	"strings"
	"time"

	"receh/internal/domain"
	"receh/internal/kit"
)

// MaxAmount caps one amount (see kit.MaxAmount).
const MaxAmount = kit.MaxAmount

// RealClock reports time in the application's configured timezone.
type RealClock struct{ Loc *time.Location }

func (c RealClock) Now() time.Time { return time.Now().In(c.Loc) }
func (c RealClock) Today() string  { return c.Now().Format(domain.DateLayout) }

// Short local names for the shared helpers in package kit.
var (
	parallel   = kit.Parallel
	notFoundOr = kit.NotFoundOr
	cleanName  = kit.CleanName
	itoa       = kit.Itoa
	itoa64     = kit.Itoa64
	formatRp   = kit.FormatRp
)

// UserHooks creates accounts: the user plus their personal workspace (which
// runs the modules' OnOrgCreated hooks). All or nothing: if the workspace
// cannot be prepared, the user is removed again.
type UserHooks struct{ orgs *OrgService }

func NewUserHooks(o *OrgService) *UserHooks { return &UserHooks{orgs: o} }

// CreateUser inserts u and, for user accounts, their personal workspace.
func (h *UserHooks) CreateUser(ctx context.Context, users domain.UserRepository, u *domain.User) error {
	if err := users.Create(ctx, u); err != nil {
		return err
	}
	if h == nil || u.Role != domain.RoleUser {
		return nil
	}
	if err := h.orgs.CreatePersonal(ctx, u); err != nil {
		_ = users.Delete(context.WithoutCancel(ctx), u.ID)
		return err
	}
	return nil
}

var emailRe = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

func normalizeEmail(s string) (string, error) {
	e := strings.ToLower(strings.TrimSpace(s))
	if len(e) > 254 || !emailRe.MatchString(e) {
		return "", domain.Invalid("email", "Format email belum benar.")
	}
	return e, nil
}

func validPassword(field, p string) error {
	if len(p) < 8 {
		return domain.Invalid(field, "Kata sandi minimal 8 karakter.")
	}
	if len(p) > 72 { // bcrypt only considers the first 72 bytes.
		return domain.Invalid(field, "Kata sandi maksimal 72 karakter.")
	}
	return nil
}
