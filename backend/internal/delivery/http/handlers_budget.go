package http

import (
	"net/http"

	"receh/internal/domain"
	"receh/internal/usecase"
)

func (s *Server) listBudgets(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	m, err := s.svc.Budgets.Month(r.Context(), u.ID, r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Server) createBudget(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.BudgetInput
	if !readJSON(w, r, &in) {
		return
	}
	b, err := s.svc.Budgets.Create(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) updateBudget(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Amount int64 `json:"amount"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	b, err := s.svc.Budgets.Update(r.Context(), u.ID, id, in.Amount)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Server) deleteBudget(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Budgets.Delete(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
