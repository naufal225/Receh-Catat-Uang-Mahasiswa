package http

import (
	"net/http"
	"strconv"

	"receh/internal/domain"
	"receh/internal/usecase"
)

// ---- Wallets ---------------------------------------------------------------

func (s *Server) listWallets(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Wallets.List(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createWallet(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.WalletInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Wallets.Create(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateWallet(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.WalletInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Wallets.Update(r.Context(), u.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteWallet(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	archived, err := s.svc.Wallets.Remove(r.Context(), u.ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"archived": archived})
}

// ---- Categories ------------------------------------------------------------

func (s *Server) listCategories(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Categories.List(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createCategory(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.CategoryInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Categories.Create(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateCategory(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.CategoryInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Categories.Update(r.Context(), u.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteCategory(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Categories.Delete(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Transactions ----------------------------------------------------------

func (s *Server) listTransactions(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	q := r.URL.Query()
	page, err := s.svc.Txs.List(r.Context(), u.ID, domain.TxFilter{
		Search:     q.Get("search"),
		Type:       domain.TxType(q.Get("type")),
		WalletID:   queryInt64(r, "walletId"),
		CategoryID: queryInt64(r, "categoryId"),
		From:       q.Get("from"),
		To:         q.Get("to"),
		Page:       queryInt(r, "page"),
		PageSize:   queryInt(r, "pageSize"),
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) createTransaction(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.TransactionInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Txs.Create(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateTransaction(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.TransactionInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Txs.Update(r.Context(), u.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteTransaction(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Txs.Delete(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Recurring -------------------------------------------------------------

func (s *Server) listRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Recurring.List(r.Context(), u.ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Server) createRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in usecase.RecurringInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Recurring.Create(r.Context(), u.ID, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Server) updateRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in usecase.RecurringInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Recurring.Update(r.Context(), u.ID, id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) setRecurringActive(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
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
	out, err := s.svc.Recurring.SetActive(r.Context(), u.ID, id, in.Active)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) deleteRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Recurring.Delete(r.Context(), u.ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Dashboard & reports ---------------------------------------------------

func (s *Server) dashboard(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	unit := r.URL.Query().Get("unit")
	if unit == "" {
		unit = "day"
	}
	out, err := s.svc.Dashboard.Build(r.Context(), u.ID, unit, queryInt(r, "offset"), queryInt64(r, "walletId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) dashboardSlices(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	q := r.URL.Query()
	out, err := s.svc.Dashboard.Slices(r.Context(), u.ID, queryInt64(r, "walletId"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) monthlyReport(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	out, err := s.svc.Reports.Monthly(r.Context(), u, r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) monthlyReportPDF(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	data, name, err := s.svc.Reports.MonthlyPDF(r.Context(), u, r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}
