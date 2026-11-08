package usecase

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"log"
	"sync"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/cache"
	"receh/internal/platform/ratelimit"

	"golang.org/x/crypto/bcrypt"
)

const sessionTTL = 30 * 24 * time.Hour

type AuthService struct {
	users    domain.UserRepository
	sessions domain.SessionRepository
	audit    *AuditRecorder
	cost     int
	// dummyHash is compared when an email is unknown, so response time does not
	// reveal which emails are registered.
	dummyHash []byte

	loginLimit    *ratelimit.Limiter
	registerLimit *ratelimit.Limiter

	// Last-seen times are buffered so a page load does not cost a database write
	// per API call. They are flushed together by Maintain.
	touchMu sync.Mutex
	touches map[int64]time.Time

	// Google sign-in; nil when not configured. See oauth.go.
	google      domain.OAuthProvider
	oauthStates *cache.TTL[string, pendingOAuth]
}

func NewAuthService(u domain.UserRepository, s domain.SessionRepository, a *AuditRecorder, bcryptCost int) *AuthService {
	dummy, err := bcrypt.GenerateFromPassword([]byte("receh-timing-equalizer"), bcryptCost)
	if err != nil {
		panic(err) // Only an invalid compile-time cost can cause this.
	}
	return &AuthService{
		users: u, sessions: s, audit: a, cost: bcryptCost, dummyHash: dummy,
		loginLimit:    ratelimit.New(8, 10*time.Minute),
		registerLimit: ratelimit.New(20, time.Hour),
		touches:       make(map[int64]time.Time),
	}
}

func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Register creates a user account with starter categories and logs them in.
func (s *AuthService) Register(ctx context.Context, name, email, password string, client domain.ClientInfo) (*domain.User, string, error) {
	if s.registerLimit.Blocked(client.IP) {
		return nil, "", domain.TooMany("Terlalu banyak pendaftaran dari jaringan ini. Coba lagi nanti.")
	}
	name, err := cleanName("name", name, 60, "Nama wajib diisi.")
	if err != nil {
		return nil, "", err
	}
	email, err = normalizeEmail(email)
	if err != nil {
		return nil, "", err
	}
	if err := validPassword("password", password); err != nil {
		return nil, "", err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.cost)
	if err != nil {
		return nil, "", err
	}
	u := &domain.User{Name: name, Email: email, PasswordHash: string(hash), Role: domain.RoleUser, Status: domain.StatusActive}
	if err := s.users.CreateWithDefaults(ctx, u, DefaultCategories()); err != nil {
		var app *domain.AppError
		if errors.As(err, &app) && app.Code == "EMAIL_TAKEN" {
			app = &domain.AppError{Kind: app.Kind, Code: app.Code, Message: app.Message, Field: "email"}
			return nil, "", app
		}
		return nil, "", err
	}
	s.registerLimit.Hit(client.IP)
	token, sess, err := s.startSession(ctx, u.ID, client)
	if err != nil {
		return nil, "", err
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventRegister, Client: client, SessionID: sess.ID})
	return u, token, nil
}

// Login verifies credentials and opens a session on this device.
func (s *AuthService) Login(ctx context.Context, email, password string, client domain.ClientInfo) (*domain.User, string, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return nil, "", err
	}
	limitKey := client.IP + "|" + email
	if s.loginLimit.Blocked(limitKey) {
		return nil, "", domain.TooMany("Terlalu banyak percobaan gagal. Coba lagi dalam 10 menit.")
	}

	u, err := s.users.ByEmail(ctx, email)
	if errors.Is(err, domain.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		s.loginLimit.Hit(limitKey)
		s.audit.Record(AuditEntry{Email: email, Event: domain.EventLoginFailed, Client: client, Detail: "Email tidak terdaftar"})
		return nil, "", domain.ErrBadCredentials
	}
	if err != nil {
		return nil, "", err
	}
	// An account created with Google has no password: compare against the dummy
	// so it takes as long as a wrong password, then refuse.
	if u.PasswordHash == "" {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		s.loginLimit.Hit(limitKey)
		s.audit.Record(AuditEntry{UserID: &u.ID, Email: email, Event: domain.EventLoginFailed, Client: client, Detail: "Akun ini masuk dengan Google"})
		return nil, "", domain.ErrBadCredentials
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		s.loginLimit.Hit(limitKey)
		s.audit.Record(AuditEntry{UserID: &u.ID, Email: email, Event: domain.EventLoginFailed, Client: client, Detail: "Kata sandi salah"})
		return nil, "", domain.ErrBadCredentials
	}
	// The suspension reason is only revealed after the password checks out.
	if u.Status == domain.StatusSuspended {
		s.audit.Record(AuditEntry{UserID: &u.ID, Email: email, Event: domain.EventLoginBlocked, Client: client, Detail: "Akun ditangguhkan"})
		return nil, "", domain.Suspended(u.StatusReason)
	}

	s.loginLimit.Reset(limitKey)
	token, sess, err := s.startSession(ctx, u.ID, client)
	if err != nil {
		return nil, "", err
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: email, Event: domain.EventLoginSuccess, Client: client, SessionID: sess.ID})
	return u, token, nil
}

func (s *AuthService) startSession(ctx context.Context, userID int64, client domain.ClientInfo) (string, *domain.Session, error) {
	token, err := newToken()
	if err != nil {
		return "", nil, err
	}
	now := time.Now().UTC()
	sess := &domain.Session{
		UserID: userID, TokenHash: HashToken(token), IP: client.IP, UserAgent: client.UserAgent,
		Device: client.Device, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(sessionTTL),
	}
	if err := s.sessions.Create(ctx, sess); err != nil {
		return "", nil, err
	}
	return token, sess, nil
}

// Authenticate resolves a cookie token to its session and user. Status is read
// fresh on every request, so an admin's freeze applies immediately.
func (s *AuthService) Authenticate(ctx context.Context, token string) (*domain.User, *domain.Session, error) {
	sess, err := s.sessions.ByTokenHash(ctx, HashToken(token))
	if errors.Is(err, domain.ErrNoRows) {
		return nil, nil, domain.ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, err
	}
	u, err := s.users.ByID(ctx, sess.UserID)
	if errors.Is(err, domain.ErrNoRows) {
		return nil, nil, domain.ErrUnauthenticated
	}
	if err != nil {
		return nil, nil, err
	}
	if u.Status == domain.StatusSuspended {
		return nil, nil, domain.ErrUnauthenticated
	}
	s.touchMu.Lock()
	s.touches[sess.ID] = time.Now().UTC()
	s.touchMu.Unlock()
	return u, sess, nil
}

func (s *AuthService) Logout(ctx context.Context, u *domain.User, sess *domain.Session, client domain.ClientInfo) error {
	if _, err := s.sessions.Revoke(ctx, u.ID, sess.ID); err != nil {
		return err
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventLogout, Client: client})
	return nil
}

func (s *AuthService) ListSessions(ctx context.Context, userID, currentID int64) ([]domain.Session, error) {
	s.flushTouches(ctx)
	list, err := s.sessions.ListActive(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range list {
		list[i].Current = list[i].ID == currentID
	}
	return list, nil
}

func (s *AuthService) RevokeSession(ctx context.Context, u *domain.User, currentID, targetID int64, client domain.ClientInfo) error {
	if targetID == currentID {
		return domain.Invalid("", "Gunakan tombol Keluar untuk perangkat yang sedang dipakai.")
	}
	ok, err := s.sessions.Revoke(ctx, u.ID, targetID)
	if err != nil {
		return err
	}
	if !ok {
		return domain.NotFound("Perangkat tidak ditemukan.")
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventSessionRevoked, Client: client, Detail: "Satu perangkat dikeluarkan"})
	return nil
}

func (s *AuthService) RevokeOtherSessions(ctx context.Context, u *domain.User, currentID int64, client domain.ClientInfo) (int64, error) {
	n, err := s.sessions.RevokeOthers(ctx, u.ID, currentID)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventSessionRevoked, Client: client,
			Detail: itoa(int(n)) + " perangkat lain dikeluarkan"})
	}
	return n, nil
}

// ChangePassword signs out every other device: if the password changed because
// it leaked, an attacker's session must not survive the change.
// An account created with Google sets its first password without an old one.
func (s *AuthService) ChangePassword(ctx context.Context, u *domain.User, currentID int64, oldPw, newPw string, client domain.ClientInfo) error {
	if u.PasswordHash != "" && bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(oldPw)) != nil {
		return domain.Invalid("currentPassword", "Kata sandi saat ini salah.")
	}
	if err := validPassword("newPassword", newPw); err != nil {
		return err
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(newPw), s.cost)
	if err != nil {
		return err
	}
	if err := s.users.UpdatePassword(ctx, u.ID, string(hash)); err != nil {
		return err
	}
	if _, err := s.sessions.RevokeOthers(ctx, u.ID, currentID); err != nil {
		return err
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventPasswordChanged, Client: client})
	return nil
}

func (s *AuthService) flushTouches(ctx context.Context) {
	s.touchMu.Lock()
	pending := s.touches
	s.touches = make(map[int64]time.Time)
	s.touchMu.Unlock()
	if err := s.sessions.Touch(ctx, pending); err != nil {
		log.Printf("auth: flush last-seen: %v", err)
	}
}

// Maintain runs until ctx ends: it flushes last-seen times and sweeps expired
// sessions and rate-limit windows. It flushes once more on the way out.
func (s *AuthService) Maintain(ctx context.Context) {
	flush := time.NewTicker(30 * time.Second)
	sweep := time.NewTicker(time.Hour)
	defer flush.Stop()
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			final, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.flushTouches(final)
			return
		case <-flush.C:
			s.flushTouches(ctx)
		case <-sweep.C:
			s.loginLimit.Sweep()
			s.registerLimit.Sweep()
			if err := s.sessions.DeleteExpired(ctx); err != nil {
				log.Printf("auth: delete expired sessions: %v", err)
			}
		}
	}
}
