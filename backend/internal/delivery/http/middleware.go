package http

import (
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/useragent"
)

const cookieName = "receh_session"

// authed is a handler that already knows who is calling.
type authed func(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session)

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request) (*domain.User, *domain.Session, bool) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		writeError(w, domain.ErrUnauthenticated)
		return nil, nil, false
	}
	u, sess, err := s.svc.Auth.Authenticate(r.Context(), c.Value)
	if err != nil {
		s.clearCookie(w)
		writeError(w, err)
		return nil, nil, false
	}
	return u, sess, true
}

func (s *Server) anyone(next authed) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if u, sess, ok := s.authenticate(w, r); ok {
			next(w, r, u, sess)
		}
	}
}

func (s *Server) userRead(next authed) http.HandlerFunc {
	return s.anyone(func(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
		if u.Role != domain.RoleUser {
			writeError(w, domain.ErrUserOnly)
			return
		}
		next(w, r, u, sess)
	})
}

func (s *Server) userWrite(next authed) http.HandlerFunc {
	return s.userRead(func(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
		if u.Status == domain.StatusFrozen {
			writeError(w, domain.ErrAccountFrozen)
			return
		}
		next(w, r, u, sess)
	})
}

func (s *Server) admin(next authed) http.HandlerFunc {
	return s.anyone(func(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
		if u.Role != domain.RoleAdmin {
			writeError(w, domain.ErrAdminOnly)
			return
		}
		next(w, r, u, sess)
	})
}

// clientInfo reads the caller's device. X-Forwarded-For is honoured only when
// configured, because otherwise any client could forge its audit log IP. The
// last entry is used: it is the one appended by the trusted proxy itself.
func (s *Server) clientInfo(r *http.Request) domain.ClientInfo {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	if s.opts.TrustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			parts := strings.Split(fwd, ",")
			if candidate := strings.TrimSpace(parts[len(parts)-1]); net.ParseIP(candidate) != nil {
				ip = candidate
			}
		}
	}
	ua := r.UserAgent()
	if len(ua) > 400 {
		ua = ua[:400]
	}
	return domain.ClientInfo{IP: ip, UserAgent: ua, Device: useragent.Describe(ua)}
}

func (s *Server) setCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", HttpOnly: true, Secure: s.opts.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: 30 * 24 * 3600,
	})
}

func (s *Server) clearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: "", Path: "/", HttpOnly: true, Secure: s.opts.CookieSecure,
		SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
}

// sameOrigin blocks cross-site state changes. Together with the SameSite cookie
// this covers CSRF without a token.
func sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if origin := r.Header.Get("Origin"); origin != "" {
				u, err := url.Parse(origin)
				if err != nil || u.Host != r.Host {
					writeJSON(w, http.StatusForbidden, errorBody{Code: "BAD_ORIGIN", Message: "Permintaan dari situs lain ditolak."})
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; font-src https://fonts.gstatic.com; img-src 'self' data:; frame-ancestors 'none'; base-uri 'self'")
		next.ServeHTTP(w, r)
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s %d %s", r.Method, r.URL.Path, rec.status, time.Since(start).Round(time.Millisecond))
		}
	})
}

func recoverPanic(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				log.Printf("panic on %s %s: %v", r.Method, r.URL.Path, v)
				writeJSON(w, http.StatusInternalServerError, errorBody{Code: "INTERNAL", Message: "Terjadi kesalahan di server."})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
