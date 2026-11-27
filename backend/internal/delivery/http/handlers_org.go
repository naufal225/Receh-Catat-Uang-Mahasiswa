package http

import (
	"net/http"

	"receh/internal/domain"
)

func (s *Server) listOrgs(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Orgs.List(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createOrg(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	o, err := s.svc.Orgs.Create(r.Context(), u, in.Name, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, o)
}

func (s *Server) renameOrg(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.svc.Orgs.Rename(r.Context(), u, id, in.Name, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteOrg(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Orgs.Delete(r.Context(), u, id, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) orgMembers(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := s.svc.Orgs.Members(r.Context(), u, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) setMemberRole(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, ok := pathInt64(w, r, "uid")
	if !ok {
		return
	}
	var in struct {
		Role domain.OrgRole `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.svc.Orgs.SetRole(r.Context(), u, id, target, in.Role, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) removeMember(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	target, ok := pathInt64(w, r, "uid")
	if !ok {
		return
	}
	if err := s.svc.Orgs.RemoveMember(r.Context(), u, id, target, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) transferOrg(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		UserID int64 `json:"userId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.svc.Orgs.TransferOwner(r.Context(), u, id, in.UserID, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) listInvites(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := s.svc.Orgs.PendingInvites(r.Context(), u, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createInvite(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Email string         `json:"email"`
		Role  domain.OrgRole `json:"role"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	res, err := s.svc.Orgs.Invite(r.Context(), u, id, in.Email, in.Role, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

func (s *Server) revokeInvite(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	iid, ok := pathInt64(w, r, "iid")
	if !ok {
		return
	}
	if err := s.svc.Orgs.RevokeInvite(r.Context(), u, id, iid, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) inviteInfo(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	inv, err := s.svc.Orgs.InviteInfo(r.Context(), u, r.PathValue("token"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, inv)
}

func (s *Server) acceptInvite(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	m, err := s.svc.Orgs.Accept(r.Context(), u, r.PathValue("token"), s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}
