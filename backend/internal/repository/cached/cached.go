// Package cached wraps repository ports with in-memory caches. Use cases see
// the same interfaces and do not know a cache exists.
//
// Only the lookups made on every authenticated request are cached: the session
// by token hash and the user by id. Every write that could make a cached entry
// wrong deletes it in the same call, so revoking a device or freezing an
// account still takes effect on the very next request.
package cached

import (
	"context"
	"sync/atomic"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/cache"
)

// epoch guards against a lost invalidation: a reader that queried the database
// before a revoke must not put the old row into the cache after the revoke
// cleared it. Readers note the epoch before querying and store only if no
// invalidation happened meanwhile.
type epoch struct{ n atomic.Uint64 }

func (e *epoch) now() uint64 { return e.n.Load() }
func (e *epoch) bump()       { e.n.Add(1) }

// ---- Sessions ---------------------------------------------------------------

type SessionRepo struct {
	domain.SessionRepository
	c     *cache.TTL[string, domain.Session]
	epoch epoch
}

func NewSessionRepo(inner domain.SessionRepository, ttl time.Duration) *SessionRepo {
	return &SessionRepo{SessionRepository: inner, c: cache.New[string, domain.Session](ttl, 10_000)}
}

// Cache exposes the underlying cache so the app can run its janitor.
func (r *SessionRepo) Cache() *cache.TTL[string, domain.Session] { return r.c }

func (r *SessionRepo) ByTokenHash(ctx context.Context, hash string) (*domain.Session, error) {
	if s, ok := r.c.Get(hash); ok {
		if time.Now().Before(s.ExpiresAt) {
			return &s, nil
		}
		r.c.Delete(hash)
	}
	seen := r.epoch.now()
	s, err := r.SessionRepository.ByTokenHash(ctx, hash)
	if err != nil {
		return nil, err
	}
	if r.epoch.now() == seen {
		r.c.Set(hash, *s)
	}
	return s, nil
}

func (r *SessionRepo) forget(match func(domain.Session) bool) {
	r.epoch.bump()
	r.c.DeleteFunc(func(_ string, s domain.Session) bool { return match(s) })
}

func (r *SessionRepo) Revoke(ctx context.Context, userID, sessionID int64) (bool, error) {
	ok, err := r.SessionRepository.Revoke(ctx, userID, sessionID)
	r.forget(func(s domain.Session) bool { return s.ID == sessionID })
	return ok, err
}

func (r *SessionRepo) RevokeOthers(ctx context.Context, userID, keepID int64) (int64, error) {
	n, err := r.SessionRepository.RevokeOthers(ctx, userID, keepID)
	r.forget(func(s domain.Session) bool { return s.UserID == userID && s.ID != keepID })
	return n, err
}

func (r *SessionRepo) RevokeAll(ctx context.Context, userID int64) error {
	err := r.SessionRepository.RevokeAll(ctx, userID)
	r.forget(func(s domain.Session) bool { return s.UserID == userID })
	return err
}

func (r *SessionRepo) DeleteExpired(ctx context.Context) error {
	err := r.SessionRepository.DeleteExpired(ctx)
	now := time.Now()
	r.forget(func(s domain.Session) bool { return !now.Before(s.ExpiresAt) })
	return err
}

// ---- Users ------------------------------------------------------------------

type UserRepo struct {
	domain.UserRepository
	c     *cache.TTL[int64, domain.User]
	epoch epoch
}

func NewUserRepo(inner domain.UserRepository, ttl time.Duration) *UserRepo {
	return &UserRepo{UserRepository: inner, c: cache.New[int64, domain.User](ttl, 10_000)}
}

func (r *UserRepo) Cache() *cache.TTL[int64, domain.User] { return r.c }

func (r *UserRepo) ByID(ctx context.Context, id int64) (*domain.User, error) {
	if u, ok := r.c.Get(id); ok {
		return &u, nil
	}
	seen := r.epoch.now()
	u, err := r.UserRepository.ByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if r.epoch.now() == seen {
		r.c.Set(id, *u)
	}
	return u, nil
}

func (r *UserRepo) forget(id int64) {
	r.epoch.bump()
	r.c.Delete(id)
}

func (r *UserRepo) UpdatePassword(ctx context.Context, id int64, hash string) error {
	err := r.UserRepository.UpdatePassword(ctx, id, hash)
	r.forget(id)
	return err
}

func (r *UserRepo) LinkGoogle(ctx context.Context, id int64, sub string) error {
	err := r.UserRepository.LinkGoogle(ctx, id, sub)
	r.forget(id)
	return err
}

func (r *UserRepo) UpdateStatus(ctx context.Context, id int64, status domain.UserStatus, reason string) error {
	err := r.UserRepository.UpdateStatus(ctx, id, status, reason)
	r.forget(id)
	return err
}
