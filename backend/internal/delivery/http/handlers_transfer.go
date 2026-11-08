package http

import (
	"net/http"

	"receh/internal/domain"
	"receh/internal/usecase"
)

func (s *Server) getTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.svc.Transfers.Get(r.Context(), u.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) createTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.TransferInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := s.svc.Transfers.Create(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Server) updateTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.TransferInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := s.svc.Transfers.Update(r.Context(), u.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Server) deleteTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Transfers.Delete(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// transferFee answers the fee to prefill for a route: the one used last time.
func (s *Server) transferFee(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	hint, err := s.svc.Transfers.LastFee(r.Context(), u.ID, queryInt64(r, "from"), queryInt64(r, "to"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hint)
}
