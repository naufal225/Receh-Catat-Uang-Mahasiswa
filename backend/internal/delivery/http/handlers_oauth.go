package http

import (
	"errors"
	"log"
	"net/http"
	"net/url"

	"receh/internal/domain"
)

// oauthCookie carries the OAuth state from /start to /callback on this browser.
// Its path limits it to those two endpoints, and SameSite=Lax still sends it on
// Google's top-level redirect back.
const (
	oauthCookie     = "receh_oauth"
	oauthCookiePath = "/api/auth/google"
)

func (s *Server) providers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.Auth.Providers())
}

// googleStart sends the browser to Google's account picker. It is a plain link
// target (a full navigation), not a fetch, because Google must show its page.
func (s *Server) googleStart(w http.ResponseWriter, r *http.Request) {
	target, state, err := s.svc.Auth.StartGoogle()
	if err != nil {
		backToSignIn(w, r, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: oauthCookie, Value: state, Path: oauthCookiePath, MaxAge: 600,
		HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, target, http.StatusFound)
}

// googleCallback is where Google returns the browser. Success opens a session
// and lands on the app (new accounts on wallet onboarding); every failure
// lands on the sign-in page with a code it knows how to explain.
func (s *Server) googleCallback(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name: oauthCookie, Value: "", Path: oauthCookiePath, MaxAge: -1,
		HttpOnly: true, Secure: s.opts.CookieSecure, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		code := "OAUTH_FAILED"
		if e == "access_denied" {
			code = "OAUTH_CANCELLED"
		}
		http.Redirect(w, r, "/masuk?oauth="+code, http.StatusFound)
		return
	}
	cookieState := ""
	if c, err := r.Cookie(oauthCookie); err == nil {
		cookieState = c.Value
	}
	res, err := s.svc.Auth.FinishGoogle(r.Context(), cookieState, q.Get("state"), q.Get("code"), s.clientInfo(r))
	if err != nil {
		backToSignIn(w, r, err)
		return
	}
	s.setCookie(w, res.Token)
	dest := "/app"
	if res.IsNew {
		dest = "/app/dompet?baru=1"
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// noticeCookie hands the sign-in page a message from the server (the admin's
// suspension reason). It is a cookie, never a URL parameter: anyone can craft
// a link with arbitrary ?text=, but no other site can set this cookie, so the
// page never shows attacker-written text. It lives for one minute and the page
// deletes it after reading.
const noticeCookie = "receh_notice"

func backToSignIn(w http.ResponseWriter, r *http.Request, err error) {
	q := url.Values{"oauth": {"OAUTH_FAILED"}}
	var app *domain.AppError
	if errors.As(err, &app) {
		q.Set("oauth", app.Code)
		if app.Code == "ACCOUNT_SUSPENDED" {
			http.SetCookie(w, &http.Cookie{
				Name: noticeCookie, Value: url.QueryEscape(app.Message), Path: "/", MaxAge: 60,
				SameSite: http.SameSiteLaxMode,
			})
		}
	} else {
		log.Printf("google sign-in: %v", err)
	}
	http.Redirect(w, r, "/masuk?"+q.Encode(), http.StatusFound)
}
