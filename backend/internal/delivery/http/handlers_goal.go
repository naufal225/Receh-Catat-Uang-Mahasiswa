package http

import (
	"net/http"
	"strconv"

	"receh/internal/domain"
	"receh/internal/usecase"
)

func (s *Server) listGoals(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Goals.List(r.Context(), u.ID, r.URL.Query().Get("archived") == "1")
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.GoalInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := s.svc.Goals.Create(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) updateGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.GoalInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := s.svc.Goals.Update(r.Context(), u.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) deleteGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Goals.Delete(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) archiveGoal(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
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
	g, err := s.svc.Goals.SetArchived(r.Context(), u.ID, id, in.Archived)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) goalContributions(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	list, err := s.svc.Goals.Contributions(r.Context(), u.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) contribute(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.ContributionInput
	if !readJSON(w, r, &in) {
		return
	}
	g, err := s.svc.Goals.Contribute(r.Context(), u.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, g)
}

func (s *Server) deleteContribution(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	cid, err := strconv.ParseInt(r.PathValue("cid"), 10, 64)
	if err != nil || cid <= 0 {
		writeError(w, domain.NotFound("Setoran tidak ditemukan."))
		return
	}
	g, err := s.svc.Goals.DeleteContribution(r.Context(), u.ID, id, cid)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}
