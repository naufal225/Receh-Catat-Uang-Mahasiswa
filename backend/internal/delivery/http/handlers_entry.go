package http

import (
	"net/http"

	"receh/internal/domain"
	"receh/internal/usecase"
)

// importBodyLimit allows a CSV of a few thousand rows inside a JSON body.
const importBodyLimit = 3 << 20

func (s *Server) quickParse(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in struct {
		Text string `json:"text"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	d, err := s.svc.Entries.Parse(r.Context(), u.ID, in.Text)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) suggestNames(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Entries.Suggest(r.Context(), u.ID, r.URL.Query().Get("q"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) importPreview(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in struct {
		WalletID int64                    `json:"walletId"`
		CSV      string                   `json:"csv"`
		Mapping  *domain.StatementMapping `json:"mapping"`
	}
	if !readJSONLimit(w, r, &in, importBodyLimit) {
		return
	}
	p, err := s.svc.Entries.Preview(r.Context(), u.ID, in.WalletID, in.CSV, in.Mapping)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) importCommit(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.ImportCommit
	if !readJSONLimit(w, r, &in, importBodyLimit) {
		return
	}
	b, err := s.svc.Entries.Commit(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, b)
}

func (s *Server) importBatches(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Entries.Batches(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) importUndo(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Entries.Undo(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
