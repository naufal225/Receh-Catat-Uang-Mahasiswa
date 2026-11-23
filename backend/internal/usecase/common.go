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

// UserHooks lets modules prepare a new account (e.g. starter categories).
// Creation is all-or-nothing: if a hook fails, the user is removed again.
type UserHooks struct {
	fns []func(ctx context.Context, userID int64) error
}

// OnUserCreated registers a hook; call it before serving.
func (h *UserHooks) OnUserCreated(fn func(ctx context.Context, userID int64) error) {
	h.fns = append(h.fns, fn)
}

// CreateUser inserts u and runs every hook for it.
func (h *UserHooks) CreateUser(ctx context.Context, users domain.UserRepository, u *domain.User) error {
	if err := users.Create(ctx, u); err != nil {
		return err
	}
	if h == nil {
		return nil
	}
	for _, fn := range h.fns {
		if err := fn(ctx, u.ID); err != nil {
			_ = users.Delete(context.WithoutCancel(ctx), u.ID)
			return err
		}
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
