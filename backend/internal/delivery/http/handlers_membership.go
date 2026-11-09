package http

import (
	"net/http"

	"receh/internal/domain"
	"receh/internal/usecase"
)

func (s *Server) membership(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	m, err := s.svc.Membership.Get(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) redeemCode(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in struct {
		Code string `json:"code"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	m, err := s.svc.Membership.Redeem(r.Context(), u, in.Code, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

// ---- Admin -------------------------------------------------------------------

func (s *Server) adminUserPremium(w http.ResponseWriter, r *http.Request, _ *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := s.svc.Membership.ListForUser(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"billingEnabled": s.svc.Membership.BillingEnabled(), "items": list})
}

func (s *Server) adminGrantPremium(w http.ResponseWriter, r *http.Request, actor *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Until string `json:"until"` // YYYY-MM-DD, empty = forever
		Note  string `json:"note"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	e, err := s.svc.Membership.Grant(r.Context(), actor, id, in.Until, in.Note, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, e)
}

func (s *Server) adminRevokePremium(w http.ResponseWriter, r *http.Request, actor *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Membership.Revoke(r.Context(), actor, id, s.clientInfo(r)); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) adminListPromos(w http.ResponseWriter, r *http.Request, _ *domain.User, _ *domain.Session) {
	list, err := s.svc.Membership.ListPromos(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"billingEnabled": s.svc.Membership.BillingEnabled(), "items": list})
}

func (s *Server) adminCreatePromo(w http.ResponseWriter, r *http.Request, actor *domain.User, _ *domain.Session) {
	var in usecase.PromoInput
	if !readJSON(w, r, &in) {
		return
	}
	p, err := s.svc.Membership.CreatePromo(r.Context(), actor, in, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) adminSetPromoActive(w http.ResponseWriter, r *http.Request, _ *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Active bool `json:"active"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.svc.Membership.SetPromoActive(r.Context(), id, in.Active); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
