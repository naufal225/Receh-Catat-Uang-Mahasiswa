package http

import (
	"net/http"

	"receh/internal/domain"
)

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var in struct{ Name, Email, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	u, token, err := s.svc.Auth.Register(r.Context(), in.Name, in.Email, in.Password, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	s.setCookie(w, token)
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	u, token, err := s.svc.Auth.Login(r.Context(), in.Email, in.Password, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	s.setCookie(w, token)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
	if err := s.svc.Auth.Logout(r.Context(), u, sess, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	s.clearCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, _ *http.Request, u *domain.User, _ *domain.Session) {
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) changePassword(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
	var in struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.svc.Auth.ChangePassword(r.Context(), u, sess.ID, in.CurrentPassword, in.NewPassword, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listSessions(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
	list, err := s.svc.Auth.ListSessions(r.Context(), u.ID, sess.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) revokeSession(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Auth.RevokeSession(r.Context(), u, sess.ID, id, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) revokeOtherSessions(w http.ResponseWriter, r *http.Request, u *domain.User, sess *domain.Session) {
	n, err := s.svc.Auth.RevokeOtherSessions(r.Context(), u, sess.ID, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"revoked": n})
}
