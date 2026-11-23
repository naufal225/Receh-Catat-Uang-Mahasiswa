// Package http is the delivery layer: it translates HTTP requests into use case
// calls and results into JSON. No business rule lives here.
package http

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"receh/internal/usecase"
)

// Services groups what the handlers call.
type Services struct {
	Auth          *usecase.AuthService
	Admin         *usecase.AdminService
	Membership    *usecase.MembershipService
	Notifications *usecase.NotificationService
	Orgs          *usecase.OrgService
}

type Options struct {
	CookieSecure bool
	TrustProxy   bool
	StaticDir    string
}

type Server struct {
	svc    Services
	opts   Options
	mounts []Mounter
}

func NewServer(svc Services, opts Options) *Server { return &Server{svc: svc, opts: opts} }

// Handler builds the route table and wraps it in the middleware chain.
//
// Route guards: anyone(...) needs any session, userRead(...) needs a user
// account, userWrite(...) additionally refuses frozen accounts, admin(...) needs
// an admin account.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/auth/providers", s.providers)
	mux.HandleFunc("GET /api/auth/google/start", s.googleStart)
	mux.HandleFunc("GET /api/auth/google/callback", s.googleCallback)
	mux.HandleFunc("POST /api/auth/logout", s.anyone(s.logout))
	mux.HandleFunc("GET /api/auth/me", s.anyone(s.me))
	mux.HandleFunc("POST /api/auth/password", s.anyone(s.changePassword))

	mux.HandleFunc("GET /api/sessions", s.anyone(s.listSessions))
	mux.HandleFunc("DELETE /api/sessions/{id}", s.anyone(s.revokeSession))
	mux.HandleFunc("DELETE /api/sessions", s.anyone(s.revokeOtherSessions))

	mux.HandleFunc("GET /api/orgs", s.userRead(s.listOrgs))
	mux.HandleFunc("POST /api/orgs", s.userWrite(s.createOrg))
	mux.HandleFunc("PUT /api/orgs/{id}", s.userWrite(s.renameOrg))
	mux.HandleFunc("DELETE /api/orgs/{id}", s.userWrite(s.deleteOrg))
	mux.HandleFunc("GET /api/orgs/{id}/members", s.userRead(s.orgMembers))
	mux.HandleFunc("PUT /api/orgs/{id}/members/{uid}", s.userWrite(s.setMemberRole))
	mux.HandleFunc("DELETE /api/orgs/{id}/members/{uid}", s.userWrite(s.removeMember))
	mux.HandleFunc("POST /api/orgs/{id}/transfer", s.userWrite(s.transferOrg))
	mux.HandleFunc("GET /api/orgs/{id}/invitations", s.userRead(s.listInvites))
	mux.HandleFunc("POST /api/orgs/{id}/invitations", s.userWrite(s.createInvite))
	mux.HandleFunc("DELETE /api/orgs/{id}/invitations/{iid}", s.userWrite(s.revokeInvite))
	mux.HandleFunc("GET /api/invitations/{token}", s.userRead(s.inviteInfo))
	mux.HandleFunc("POST /api/invitations/{token}/accept", s.userWrite(s.acceptInvite))

	mux.HandleFunc("GET /api/notifications", s.userRead(s.inbox))
	mux.HandleFunc("GET /api/notifications/unread", s.userRead(s.unreadCount))
	mux.HandleFunc("POST /api/notifications/read", s.userRead(s.markRead))
	mux.HandleFunc("GET /api/notifications/prefs", s.userRead(s.notificationPrefs))
	mux.HandleFunc("PUT /api/notifications/prefs", s.userWrite(s.saveNotificationPrefs))

	mux.HandleFunc("GET /api/membership", s.orgRead(s.membership))
	mux.HandleFunc("POST /api/membership/redeem", s.orgWrite(s.redeemCode))

	mux.HandleFunc("GET /api/admin/users", s.admin(s.adminListUsers))
	mux.HandleFunc("POST /api/admin/users", s.admin(s.adminCreateUser))
	mux.HandleFunc("POST /api/admin/users/{id}/status", s.admin(s.adminSetStatus))
	mux.HandleFunc("GET /api/admin/audit-logs", s.admin(s.adminAuditLogs))
	mux.HandleFunc("GET /api/admin/users/{id}/premium", s.admin(s.adminUserPremium))
	mux.HandleFunc("POST /api/admin/users/{id}/premium", s.admin(s.adminGrantPremium))
	mux.HandleFunc("DELETE /api/admin/premium/{id}", s.admin(s.adminRevokePremium))
	mux.HandleFunc("GET /api/admin/promo-codes", s.admin(s.adminListPromos))
	mux.HandleFunc("POST /api/admin/promo-codes", s.admin(s.adminCreatePromo))
	mux.HandleFunc("POST /api/admin/promo-codes/{id}/active", s.admin(s.adminSetPromoActive))

	g := s.guards()
	for _, m := range s.mounts {
		m(mux, g)
	}

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, errorBody{Code: "NOT_FOUND", Message: "Endpoint tidak ditemukan."})
	})
	if s.opts.StaticDir != "" {
		mux.Handle("/", spaHandler(s.opts.StaticDir))
	}
	return recoverPanic(logRequests(securityHeaders(sameOrigin(mux))))
}

// spaHandler serves the built frontend, answering unknown paths with index.html
// so client-side routes survive a page reload.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Join(dir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
