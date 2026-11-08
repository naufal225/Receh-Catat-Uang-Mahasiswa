package usecase

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"receh/internal/domain"
	"receh/internal/platform/cache"
)

// oauthStateTTL bounds how long someone may stay on Google's account picker.
const oauthStateTTL = 10 * time.Minute

// pendingOAuth is what the server remembers between sending the browser to
// Google and Google sending it back. The PKCE verifier never leaves the server.
type pendingOAuth struct {
	verifier string
}

// EnableGoogle turns on "Masuk dengan Google". Without it the feature reports
// itself disabled and the button is hidden.
func (s *AuthService) EnableGoogle(p domain.OAuthProvider) {
	s.google = p
	s.oauthStates = cache.New[string, pendingOAuth](oauthStateTTL, 10_000)
}

// OAuthStates exposes the pending-state cache so the app can run its janitor.
func (s *AuthService) OAuthStates() *cache.TTL[string, pendingOAuth] { return s.oauthStates }

// Providers lists which external sign-in methods are available.
func (s *AuthService) Providers() map[string]bool {
	return map[string]bool{"google": s.google != nil}
}

// StartGoogle returns the Google URL to send the browser to and the state the
// browser must carry back in a cookie.
func (s *AuthService) StartGoogle() (redirectURL, state string, err error) {
	if s.google == nil {
		return "", "", domain.ErrOAuthDisabled
	}
	if state, err = newToken(); err != nil {
		return "", "", err
	}
	verifier, err := newToken() // 43 chars of base64url: a valid RFC 7636 verifier.
	if err != nil {
		return "", "", err
	}
	s.oauthStates.Set(state, pendingOAuth{verifier: verifier})
	sum := sha256.Sum256([]byte(verifier))
	return s.google.AuthURL(state, base64.RawURLEncoding.EncodeToString(sum[:])), state, nil
}

// GoogleResult says where the person should land after signing in.
type GoogleResult struct {
	User  *domain.User
	Token string
	IsNew bool
}

// FinishGoogle completes the flow when Google redirects back. The state must
// match both the cookie set on this browser and an unexpired, unused entry on
// the server: that is what stops a forged callback (login CSRF) and a replay.
func (s *AuthService) FinishGoogle(ctx context.Context, cookieState, state, code string, client domain.ClientInfo) (*GoogleResult, error) {
	if s.google == nil {
		return nil, domain.ErrOAuthDisabled
	}
	if state == "" || cookieState == "" || state != cookieState || code == "" {
		return nil, domain.ErrOAuthFailed
	}
	pending, ok := s.oauthStates.Take(state) // One use only, success or not.
	if !ok {
		return nil, domain.ErrOAuthFailed
	}
	id, err := s.google.Exchange(ctx, code, pending.verifier)
	if err != nil {
		log.Printf("google sign-in: %v", err) // Details stay in the server log.
		return nil, domain.ErrOAuthFailed
	}
	if !id.EmailVerified {
		return nil, domain.ErrOAuthUnverified
	}
	email, err := normalizeEmail(id.Email)
	if err != nil {
		return nil, domain.ErrOAuthFailed
	}

	u, isNew, err := s.googleAccount(ctx, id, email, client)
	if err != nil {
		return nil, err
	}
	if u.Role == domain.RoleAdmin {
		s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventLoginBlocked, Client: client, Detail: "Admin mencoba masuk dengan Google"})
		return nil, domain.ErrOAuthAdmin
	}
	if u.Status == domain.StatusSuspended {
		s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventLoginBlocked, Client: client, Detail: "Akun ditangguhkan (Google)"})
		return nil, domain.Suspended(u.StatusReason)
	}

	token, sess, err := s.startSession(ctx, u.ID, client)
	if err != nil {
		return nil, err
	}
	event := domain.EventLoginSuccess
	if isNew {
		event = domain.EventRegister
	}
	s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: event, Client: client, SessionID: sess.ID, Detail: "Google"})
	return &GoogleResult{User: u, Token: token, IsNew: isNew}, nil
}

// googleAccount finds or creates the account for a Google identity:
//  1. already linked by Google's stable subject id: that account;
//  2. same verified email as an existing account: link Google to it;
//  3. otherwise: a new account with no password and the starter categories.
func (s *AuthService) googleAccount(ctx context.Context, id *domain.OAuthIdentity, email string, client domain.ClientInfo) (*domain.User, bool, error) {
	u, err := s.users.ByGoogleSub(ctx, id.Subject)
	if err == nil {
		return u, false, nil
	}
	if !errors.Is(err, domain.ErrNoRows) {
		return nil, false, err
	}

	u, err = s.users.ByEmail(ctx, email)
	if err == nil {
		if u.Role == domain.RoleAdmin {
			return u, false, nil // Refused by the caller; never link an admin.
		}
		if err := s.users.LinkGoogle(ctx, u.ID, id.Subject); err != nil {
			return nil, false, err
		}
		u.GoogleSub, u.GoogleLinked = id.Subject, true
		s.audit.Record(AuditEntry{UserID: &u.ID, Email: u.Email, Event: domain.EventGoogleLinked, Client: client, Detail: "Akun Google ditautkan"})
		return u, false, nil
	}
	if !errors.Is(err, domain.ErrNoRows) {
		return nil, false, err
	}

	if s.registerLimit.Blocked(client.IP) {
		return nil, false, domain.TooMany("Terlalu banyak pendaftaran dari jaringan ini. Coba lagi nanti.")
	}
	u = &domain.User{
		Name: googleName(id.Name, email), Email: email, GoogleSub: id.Subject,
		Role: domain.RoleUser, Status: domain.StatusActive,
	}
	if err := s.users.CreateWithDefaults(ctx, u, DefaultCategories()); err != nil {
		return nil, false, err
	}
	u.GoogleLinked = true
	s.registerLimit.Hit(client.IP)
	return u, true, nil
}

// googleName keeps the profile name within the app's limit, falling back to
// the part of the email before "@".
func googleName(name, email string) string {
	name = strings.Join(strings.Fields(name), " ")
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	if utf8.RuneCountInString(name) > 60 {
		name = string([]rune(name)[:60])
	}
	return name
}
