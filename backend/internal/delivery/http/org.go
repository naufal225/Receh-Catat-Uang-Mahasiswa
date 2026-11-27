package http

import (
	"context"
	"net/http"
	"strconv"

	"receh/internal/domain"
)

// OrgHeader names the workspace a request acts in. Without it, the account's
// personal workspace is used. The server checks membership on every request,
// so the header can never grant access.
const OrgHeader = "X-Org-Id"

type orgCtxKey struct{}

// OrgFrom returns the workspace of a request that passed an org-aware guard
// (Guards.Read / Guards.Write).
func OrgFrom(ctx context.Context) domain.OrgContext {
	o, _ := ctx.Value(orgCtxKey{}).(domain.OrgContext)
	return o
}

func (s *Server) resolveOrg(w http.ResponseWriter, r *http.Request, u *domain.User) (*http.Request, bool) {
	var id int64
	if h := r.Header.Get(OrgHeader); h != "" {
		v, err := strconv.ParseInt(h, 10, 64)
		if err != nil || v <= 0 {
			writeError(w, domain.Invalid("", "Workspace tidak valid."))
			return r, false
		}
		id = v
	}
	o, err := s.svc.Orgs.Resolve(r.Context(), u.ID, id)
	if err != nil {
		writeError(w, err)
		return r, false
	}
	return r.WithContext(context.WithValue(r.Context(), orgCtxKey{}, o)), true
}

// orgRead is userRead plus the workspace the request acts in.
func (s *Server) orgRead(next authed) http.HandlerFunc {
	return s.userRead(func(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
		if r, ok := s.resolveOrg(w, r, u); ok {
			next(w, r, u, sess)
		}
	})
}

// orgWrite is userWrite (frozen accounts refused) plus the workspace.
func (s *Server) orgWrite(next authed) http.HandlerFunc {
	return s.userWrite(func(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
		if r, ok := s.resolveOrg(w, r, u); ok {
			next(w, r, u, sess)
		}
	})
}
