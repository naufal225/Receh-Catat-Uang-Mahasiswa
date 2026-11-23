package api

import (
	"net/http"

	"receh/internal/domain"
	"receh/internal/modules/finance/service"
)

func (s *Handlers) listBudgets(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	m, err := s.svc.Budgets.Month(r.Context(), orgID(r), r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, m)
}

func (s *Handlers) createBudget(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in service.BudgetInput
	if !readJSON(w, r, &in) {
		return
	}
	b, err := s.svc.Budgets.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Handlers) updateBudget(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
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
	b, err := s.svc.Budgets.Update(r.Context(), orgID(r), id, in.Amount)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}

func (s *Handlers) deleteBudget(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Budgets.Delete(r.Context(), orgID(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
