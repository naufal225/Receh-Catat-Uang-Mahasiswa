package api

import (
	"net/http"
	"strconv"

	"receh/internal/domain"
	"receh/internal/modules/finance/service"
)

func (s *Handlers) listGoals(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Goals.List(r.Context(), orgID(r), r.URL.Query().Get("archived") == "1")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Handlers) createGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in service.GoalInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := s.svc.Goals.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Handlers) updateGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.GoalInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := s.svc.Goals.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Handlers) deleteGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Goals.Delete(r.Context(), orgID(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Handlers) archiveGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in struct {
		Archived bool `json:"archived"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	g, err := s.svc.Goals.SetArchived(r.Context(), orgID(r), id, in.Archived)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Handlers) goalContributions(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := s.svc.Goals.Contributions(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Handlers) contribute(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.ContributionInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := s.svc.Goals.Contribute(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Handlers) deleteContribution(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || cid <= 0 {
		writeError(w, domain.NotFound("Setoran tidak ditemukan."))
		return
	}
	g, err := s.svc.Goals.DeleteContribution(r.Context(), orgID(r), id, cid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
