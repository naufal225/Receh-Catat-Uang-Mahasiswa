package http

import (
	"net/http"

	"receh/internal/domain"
)

func (s *Server) adminListUsers(w http.ResponseWriter, r *http.Request, _ *domain.User, _ *domain.Session) {
	q := r.URL.Query()
	page, err := s.svc.Admin.ListUsers(r.Context(), domain.UserFilter{
		Search: q.Get("search"), Status: q.Get("status"), Page: queryInt(r, "page"), PageSize: queryInt(r, "pageSize"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) adminCreateUser(w http.ResponseWriter, r *http.Request, actor *domain.User, _ *domain.Session) {
	var in struct{ Name, Email, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	u, err := s.svc.Admin.CreateUser(r.Context(), actor, in.Name, in.Email, in.Password, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) adminSetStatus(w http.ResponseWriter, r *http.Request, actor *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Status domain.UserStatus `json:"status"`
		Reason string            `json:"reason"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	u, err := s.svc.Admin.SetStatus(r.Context(), actor, id, in.Status, in.Reason, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) adminAuditLogs(w http.ResponseWriter, r *http.Request, _ *domain.User, _ *domain.Session) {
	q := r.URL.Query()
	page, err := s.svc.Admin.AuditLogs(r.Context(), domain.AuditFilter{
		Search: q.Get("search"), Event: q.Get("event"), Page: queryInt(r, "page"), PageSize: queryInt(r, "pageSize"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}
