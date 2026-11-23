package api

import (
	"net/http"
	"strconv"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
	"receh/internal/modules/finance/service"
)

// ---- Wallets ---------------------------------------------------------------

func (s *Handlers) listWallets(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Wallets.List(r.Context(), orgID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Handlers) createWallet(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in service.WalletInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Wallets.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Handlers) updateWallet(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.WalletInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Wallets.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) deleteWallet(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	archived, err := s.svc.Wallets.Remove(r.Context(), orgID(r), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"archived": archived})
}

// ---- Categories ------------------------------------------------------------

func (s *Handlers) listCategories(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Categories.List(r.Context(), orgID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Handlers) createCategory(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in service.CategoryInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Categories.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Handlers) updateCategory(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.CategoryInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Categories.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) deleteCategory(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Categories.Delete(r.Context(), orgID(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Transactions ----------------------------------------------------------

func (s *Handlers) listTransactions(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	q := r.URL.Query()
	page, err := s.svc.Txs.List(r.Context(), orgID(r), model.TxFilter{
		Search:     q.Get("search"),
		Type:       model.TxType(q.Get("type")),
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

func (s *Handlers) createTransaction(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in service.TransactionInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Txs.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Handlers) updateTransaction(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.TransactionInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Txs.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) deleteTransaction(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Txs.Delete(r.Context(), orgID(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Recurring -------------------------------------------------------------

func (s *Handlers) listRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	list, err := s.svc.Recurring.List(r.Context(), orgID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

func (s *Handlers) createRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in service.RecurringInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Recurring.Create(r.Context(), orgID(r), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *Handlers) updateRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in service.RecurringInput
	if !readJSON(w, r, &in) {
		return
	}
	out, err := s.svc.Recurring.Update(r.Context(), orgID(r), id, in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) setRecurringActive(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
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
	out, err := s.svc.Recurring.SetActive(r.Context(), orgID(r), id, in.Active)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) deleteRecurring(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Recurring.Delete(r.Context(), orgID(r), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Dashboard & reports ---------------------------------------------------

func (s *Handlers) dashboard(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	unit := r.URL.Query().Get("unit")
	if unit == "" {
		unit = "day"
	}
	out, err := s.svc.Dashboard.Build(r.Context(), orgID(r), unit, queryInt(r, "offset"), queryInt64(r, "walletId"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) dashboardSlices(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	q := r.URL.Query()
	out, err := s.svc.Dashboard.Slices(r.Context(), orgID(r), queryInt64(r, "walletId"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) monthlyReport(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	out, err := s.svc.Reports.Monthly(r.Context(), orgID(r), u.Name, r.URL.Query().Get("month"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Handlers) monthlyReportPDF(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	data, name, err := s.svc.Reports.MonthlyPDF(r.Context(), orgID(r), u.Name, r.URL.Query().Get("month"))
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
