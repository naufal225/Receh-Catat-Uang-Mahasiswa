package api

import (
	"net/http"

	"receh/internal/domain"
	"receh/internal/modules/finance/service"
)

func (s *Handlers) getTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.svc.Transfers.Get(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Handlers) createTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in service.TransferInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := s.svc.Transfers.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (s *Handlers) updateTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.TransferInput
	if !readJSON(w, r, &in) {
		return
	}
	t, err := s.svc.Transfers.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, t)
}

func (s *Handlers) deleteTransfer(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Transfers.Delete(r.Context(), orgID(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// transferFee answers the fee to prefill for a route: the one used last time.
func (s *Handlers) transferFee(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	hint, err := s.svc.Transfers.LastFee(r.Context(), orgID(r), queryInt64(r, "from"), queryInt64(r, "to"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hint)
}
