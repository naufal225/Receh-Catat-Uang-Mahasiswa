package app_test

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"receh/internal/app"
	"receh/internal/domain"
	"receh/internal/platform/googleoauth"
)

const (
	testClientID     = "test-client.apps.googleusercontent.com"
	testClientSecret = "test-secret"
	testRedirect     = "http://receh.test/api/auth/google/callback"
)

// fakeGoogle plays Google's token and userinfo endpoints. It checks what a
// real Google would: the client secret, the redirect URI and that the PKCE
// verifier hashes to the challenge sent with the authorization request.
type fakeGoogle struct {
	t   *testing.T
	srv *httptest.Server
	mu  sync.Mutex
	// code → what Google knows about it.
	codes  map[string]fakeGrant
	tokens map[string]domain.OAuthIdentity
}

type fakeGrant struct {
	challenge string
	identity  domain.OAuthIdentity
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	g := &fakeGoogle{t: t, codes: map[string]fakeGrant{}, tokens: map[string]domain.OAuthIdentity{}}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		fail := func(code string) {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": code})
		}
		if r.Form.Get("client_id") != testClientID || r.Form.Get("client_secret") != testClientSecret {
			fail("invalid_client")
			return
		}
		if r.Form.Get("redirect_uri") != testRedirect || r.Form.Get("grant_type") != "authorization_code" {
			fail("invalid_request")
			return
		}
		g.mu.Lock()
		grant, ok := g.codes[r.Form.Get("code")]
		delete(g.codes, r.Form.Get("code"))
		g.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != grant.challenge {
			fail("invalid_grant")
			return
		}
		token := "at-" + r.Form.Get("code")
		g.mu.Lock()
		g.tokens[token] = grant.identity
		g.mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": 3600})
	})
	mux.HandleFunc("GET /userinfo", func(w http.ResponseWriter, r *http.Request) {
		g.mu.Lock()
		id, ok := g.tokens[strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")]
		g.mu.Unlock()
		if !ok {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"invalid_token"}`))
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"sub": id.Subject, "email": id.Email, "email_verified": id.EmailVerified, "name": id.Name,
		})
	})
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func (g *fakeGoogle) provider() *googleoauth.Provider {
	p := googleoauth.New(testClientID, testClientSecret, testRedirect)
	p.AuthURLBase = g.srv.URL + "/auth"
	p.TokenURL = g.srv.URL + "/token"
	p.UserinfoURL = g.srv.URL + "/userinfo"
	return p
}

// approve records that the person picked this identity on Google's page for
// the given authorization request, and returns the code Google would send back.
func (g *fakeGoogle) approve(authURL *url.URL, id domain.OAuthIdentity) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	code := fmt.Sprintf("code-%d", len(g.codes)+len(g.tokens)+1)
	g.codes[code] = fakeGrant{challenge: authURL.Query().Get("code_challenge"), identity: id}
	return code
}

func newGoogleEnv(t *testing.T) (*env, *fakeGoogle) {
	g := newFakeGoogle(t)
	e := newEnv(t, func(p *app.Params) { p.Google = g.provider() })
	return e, g
}

// raw sends a request without following redirects, so a test can inspect them.
func (c *client) raw(method, path string) *http.Response {
	c.e.t.Helper()
	hc := *c.hc
	hc.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, _ := http.NewRequest(method, c.e.srv.URL+path, nil)
	if c.ua != "" {
		req.Header.Set("User-Agent", c.ua)
	}
	resp, err := hc.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	resp.Body.Close()
	return resp
}

// start begins a Google sign-in and returns Google's authorization URL.
func (c *client) startGoogle() *url.URL {
	c.e.t.Helper()
	resp := c.raw("GET", "/api/auth/google/start")
	if resp.StatusCode != http.StatusFound {
		c.e.t.Fatalf("start: status %d", resp.StatusCode)
	}
	u, err := url.Parse(resp.Header.Get("Location"))
	if err != nil {
		c.e.t.Fatal(err)
	}
	return u
}

// callback returns Google's redirect to the app and where the app sends the browser.
func (c *client) callback(state, code string) string {
	c.e.t.Helper()
	resp := c.raw("GET", "/api/auth/google/callback?"+url.Values{"state": {state}, "code": {code}}.Encode())
	if resp.StatusCode != http.StatusFound {
		c.e.t.Fatalf("callback: status %d", resp.StatusCode)
	}
	return resp.Header.Get("Location")
}

// signIn runs the whole flow for one identity and returns the final redirect.
func (c *client) signInWithGoogle(g *fakeGoogle, id domain.OAuthIdentity) string {
	c.e.t.Helper()
	auth := c.startGoogle()
	return c.callback(auth.Query().Get("state"), g.approve(auth, id))
}

func gid(sub, email string) domain.OAuthIdentity {
	return domain.OAuthIdentity{Subject: sub, Email: email, EmailVerified: true, Name: "Rina Mahasiswi"}
}

func TestGoogleSignUpCreatesAccountAndSession(t *testing.T) {
	e, g := newGoogleEnv(t)
	c := e.client("Mozilla/5.0 (Linux; Android 14) Chrome/120.0 Mobile Safari/537.36")

	if p := must[map[string]bool](c, 200, "GET", "/api/auth/providers", nil); !p["google"] {
		t.Fatalf("providers = %v", p)
	}
	auth := c.startGoogle()
	q := auth.Query()
	for k, want := range map[string]string{
		"client_id": testClientID, "redirect_uri": testRedirect, "response_type": "code",
		"code_challenge_method": "S256", "scope": "openid email profile",
	} {
		if q.Get(k) != want {
			t.Errorf("auth URL %s = %q, want %q", k, q.Get(k), want)
		}
	}
	if len(q.Get("state")) < 40 || len(q.Get("code_challenge")) != 43 {
		t.Fatalf("weak state or challenge: %v", q)
	}
	if strings.Contains(auth.String(), testClientSecret) {
		t.Fatal("client secret leaked into the browser redirect")
	}

	dest := c.callback(q.Get("state"), g.approve(auth, gid("sub-rina", "Rina@Gmail.com")))
	if dest != "/app/dompet?baru=1" {
		t.Fatalf("new account lands on %q, want wallet onboarding", dest)
	}
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	if me.Email != "rina@gmail.com" || me.Name != "Rina Mahasiswi" || !me.GoogleLinked || me.HasPassword {
		t.Fatalf("me = %+v", me)
	}
	if cats := must[[]domain.Category](c, 200, "GET", "/api/categories", nil); len(cats) == 0 {
		t.Fatal("new Google account has no starter categories")
	}
	sessions := must[[]domain.Session](c, 200, "GET", "/api/sessions", nil)
	if len(sessions) != 1 || sessions[0].Device != "Chrome di Android" {
		t.Fatalf("sessions = %+v", sessions)
	}

	// Signing in again with the same Google account reuses it.
	again := e.client("")
	if dest := again.signInWithGoogle(g, gid("sub-rina", "rina@gmail.com")); dest != "/app" {
		t.Fatalf("returning user lands on %q", dest)
	}
	if u := must[domain.User](again, 200, "GET", "/api/auth/me", nil); u.ID != me.ID {
		t.Fatalf("second sign-in made another account: %d vs %d", u.ID, me.ID)
	}

	// Audit log shows both, tagged Google.
	logs := must[domain.Page[domain.AuditLog]](e.admin(), 200, "GET", "/api/admin/audit-logs?search=rina@gmail.com", nil)
	events := map[domain.AuditEvent]bool{}
	for _, l := range logs.Items {
		if l.Detail == "Google" {
			events[l.Event] = true
		}
	}
	if !events[domain.EventRegister] || !events[domain.EventLoginSuccess] {
		t.Fatalf("audit events %v", events)
	}
}

func TestGoogleLinksExistingPasswordAccount(t *testing.T) {
	e, g := newGoogleEnv(t)
	pw := e.client("")
	pw.expect(201, "POST", "/api/auth/register", M{"name": "Dimas", "email": "dimas@gmail.com", "password": "password-123"})
	before := must[domain.User](pw, 200, "GET", "/api/auth/me", nil)

	c := e.client("")
	if dest := c.signInWithGoogle(g, gid("sub-dimas", "dimas@gmail.com")); dest != "/app" {
		t.Fatalf("linked account lands on %q", dest)
	}
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	if me.ID != before.ID || !me.GoogleLinked || !me.HasPassword || me.Name != "Dimas" {
		t.Fatalf("after link: %+v", me)
	}
	// The password still works, and the cached user of the open session sees the link.
	e.client("").expect(200, "POST", "/api/auth/login", M{"email": "dimas@gmail.com", "password": "password-123"})
	if u := must[domain.User](pw, 200, "GET", "/api/auth/me", nil); !u.GoogleLinked {
		t.Fatal("cached user did not pick up the Google link")
	}
	logs := must[domain.Page[domain.AuditLog]](e.admin(), 200, "GET", "/api/admin/audit-logs?event=google_linked", nil)
	if logs.Total != 1 {
		t.Fatalf("%d google_linked audit rows", logs.Total)
	}

	// The same email arriving with a different Google account cannot take it over.
	other := e.client("")
	if dest := other.signInWithGoogle(g, gid("sub-impostor", "dimas@gmail.com")); !strings.Contains(dest, "oauth=OAUTH_FAILED") {
		t.Fatalf("second Google account for a linked email: %q", dest)
	}
}

func TestGoogleRejectsForgedReplayedAndUnverified(t *testing.T) {
	e, g := newGoogleEnv(t)
	c := e.client("")

	// No state cookie: a callback forged on another site (login CSRF).
	auth := c.startGoogle()
	code := g.approve(auth, gid("sub-x", "x@gmail.com"))
	stranger := e.client("")
	if dest := stranger.callback(auth.Query().Get("state"), code); !strings.Contains(dest, "OAUTH_FAILED") {
		t.Fatalf("callback without the state cookie: %q", dest)
	}
	// Wrong state with a valid cookie.
	auth = c.startGoogle()
	if dest := c.callback("not-the-state", g.approve(auth, gid("sub-x", "x@gmail.com"))); !strings.Contains(dest, "OAUTH_FAILED") {
		t.Fatalf("wrong state: %q", dest)
	}
	// A state is single use: the second callback with it fails even with a fresh code.
	auth = c.startGoogle()
	state := auth.Query().Get("state")
	if dest := c.callback(state, g.approve(auth, gid("sub-x", "x@gmail.com"))); dest != "/app/dompet?baru=1" {
		t.Fatalf("first callback: %q", dest)
	}
	replay := e.client("")
	replay.hc.Jar.SetCookies(mustURL(e.srv.URL+"/api/auth/google/"), []*http.Cookie{{Name: "receh_oauth", Value: state, Path: "/api/auth/google"}})
	if dest := replay.callback(state, g.approve(auth, gid("sub-x", "x@gmail.com"))); !strings.Contains(dest, "OAUTH_FAILED") {
		t.Fatalf("replayed state: %q", dest)
	}
	// A code redeemed with a verifier that does not match the challenge (PKCE).
	auth = c.startGoogle()
	g.mu.Lock()
	g.codes["code-pkce"] = fakeGrant{challenge: "not-the-challenge", identity: gid("sub-y", "y@gmail.com")}
	g.mu.Unlock()
	if dest := c.callback(auth.Query().Get("state"), "code-pkce"); !strings.Contains(dest, "OAUTH_FAILED") {
		t.Fatalf("PKCE mismatch: %q", dest)
	}
	// Unverified email.
	unverified := gid("sub-z", "z@gmail.com")
	unverified.EmailVerified = false
	if dest := e.client("").signInWithGoogle(g, unverified); !strings.Contains(dest, "OAUTH_EMAIL_UNVERIFIED") {
		t.Fatalf("unverified email: %q", dest)
	}
	// The person cancelled on Google's page.
	resp := c.raw("GET", "/api/auth/google/callback?error=access_denied&state=x")
	if loc := resp.Header.Get("Location"); loc != "/masuk?oauth=OAUTH_CANCELLED" {
		t.Fatalf("cancel: %q", loc)
	}
	// None of the failures opened a session for the forged/cancelled clients.
	stranger.expect(401, "GET", "/api/auth/me", nil)
}

func TestGoogleRespectsAccountStatusAndRole(t *testing.T) {
	e, g := newGoogleEnv(t)
	admin := e.admin()

	// Admin accounts never sign in with Google, and are never linked.
	if dest := e.client("").signInWithGoogle(g, gid("sub-admin", adminEmail)); !strings.Contains(dest, "OAUTH_ADMIN") {
		t.Fatalf("admin via Google: %q", dest)
	}

	c := e.client("")
	c.signInWithGoogle(g, gid("sub-sus", "sus@gmail.com"))
	me := must[domain.User](c, 200, "GET", "/api/auth/me", nil)
	status := fmt.Sprintf("/api/admin/users/%d/status", me.ID)

	admin.expect(200, "POST", status, M{"status": "suspended", "reason": "Laporan penipuan"})
	blocked := e.client("")
	dest := blocked.signInWithGoogle(g, gid("sub-sus", "sus@gmail.com"))
	if dest != "/masuk?oauth=ACCOUNT_SUSPENDED" {
		t.Fatalf("suspended via Google: %q (no free text may travel in the URL)", dest)
	}
	// The admin's reason arrives in a short-lived cookie only this server can set.
	var notice string
	for _, ck := range blocked.hc.Jar.Cookies(mustURL(e.srv.URL + "/masuk")) {
		if ck.Name == "receh_notice" {
			notice, _ = url.QueryUnescape(ck.Value)
		}
	}
	if !strings.Contains(notice, "Laporan penipuan") {
		t.Fatalf("suspension notice cookie = %q", notice)
	}
	blocked.expect(401, "GET", "/api/auth/me", nil)

	admin.expect(200, "POST", status, M{"status": "frozen", "reason": "Verifikasi"})
	frozen := e.client("")
	if dest := frozen.signInWithGoogle(g, gid("sub-sus", "sus@gmail.com")); dest != "/app" {
		t.Fatalf("frozen account should still sign in (read-only): %q", dest)
	}
	frozen.expect(200, "GET", "/api/wallets", nil)
	frozen.expect(403, "POST", "/api/wallets", M{"name": "BCA", "type": "bank", "color": "c1", "balance": 0})
}

func TestGoogleOnlyAccountCanCreatePassword(t *testing.T) {
	e, g := newGoogleEnv(t)
	c := e.client("")
	c.signInWithGoogle(g, gid("sub-sandi", "sandi@gmail.com"))

	// No password yet: password sign-in is refused like any wrong password.
	e.client("").expect(401, "POST", "/api/auth/login", M{"email": "sandi@gmail.com", "password": "password-123"})

	// The first password needs no current password.
	c.expect(204, "POST", "/api/auth/password", M{"currentPassword": "", "newPassword": "password-baru-1"})
	if me := must[domain.User](c, 200, "GET", "/api/auth/me", nil); !me.HasPassword {
		t.Fatal("hasPassword still false after creating one")
	}
	e.client("").expect(200, "POST", "/api/auth/login", M{"email": "sandi@gmail.com", "password": "password-baru-1"})
	// From now on changing it needs the current one.
	c.expect(400, "POST", "/api/auth/password", M{"currentPassword": "", "newPassword": "password-baru-2"})
}

func TestGoogleDisabledWithoutConfig(t *testing.T) {
	e := newEnv(t)
	c := e.client("")
	if p := must[map[string]bool](c, 200, "GET", "/api/auth/providers", nil); p["google"] {
		t.Fatalf("providers = %v", p)
	}
	resp := c.raw("GET", "/api/auth/google/start")
	if loc := resp.Header.Get("Location"); resp.StatusCode != http.StatusFound || loc != "/masuk?oauth=OAUTH_DISABLED" {
		t.Fatalf("start when disabled: %d %q", resp.StatusCode, loc)
	}
}

func mustURL(s string) *url.URL {
	u, err := url.Parse(s)
	if err != nil {
		panic(err)
	}
	return u
}
