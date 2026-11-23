// Package api is the finance module's HTTP layer. It mounts its routes on the
// core server and uses the core's guards and JSON helpers.
package api

import (
	"net/http"

	core "receh/internal/delivery/http"
	"receh/internal/modules/finance/service"
)

// Services groups what the finance handlers call.
type Services struct {
	Wallets    *service.WalletService
	Categories *service.CategoryService
	Txs        *service.TransactionService
	Transfers  *service.TransferService
	Recurring  *service.RecurringService
	Dashboard  *service.DashboardService
	Reports    *service.ReportService
	Budgets    *service.BudgetService
	Goals      *service.GoalService
	Entries    *service.EntryService
}

type Handlers struct{ svc Services }

func New(svc Services) *Handlers { return &Handlers{svc: svc} }

var (
	writeJSON     = core.WriteJSON
	writeError    = core.WriteError
	readJSON      = core.ReadJSON
	readJSONLimit = core.ReadJSONLimit
	pathID        = core.PathID
	queryInt      = core.QueryInt
	queryInt64    = core.QueryInt64
)

// Mount registers every finance route.
func (s *Handlers) Mount(mux *http.ServeMux, g core.Guards) {
	mux.HandleFunc("GET /api/wallets", g.Read(s.listWallets))
	mux.HandleFunc("POST /api/wallets", g.Write(s.createWallet))
	mux.HandleFunc("PUT /api/wallets/{id}", g.Write(s.updateWallet))
	mux.HandleFunc("DELETE /api/wallets/{id}", g.Write(s.deleteWallet))
	mux.HandleFunc("GET /api/categories", g.Read(s.listCategories))
	mux.HandleFunc("POST /api/categories", g.Write(s.createCategory))
	mux.HandleFunc("PUT /api/categories/{id}", g.Write(s.updateCategory))
	mux.HandleFunc("DELETE /api/categories/{id}", g.Write(s.deleteCategory))
	mux.HandleFunc("GET /api/transactions", g.Read(s.listTransactions))
	mux.HandleFunc("POST /api/transactions", g.Write(s.createTransaction))
	mux.HandleFunc("PUT /api/transactions/{id}", g.Write(s.updateTransaction))
	mux.HandleFunc("DELETE /api/transactions/{id}", g.Write(s.deleteTransaction))
	mux.HandleFunc("GET /api/transfers/last-fee", g.Read(s.transferFee))
	mux.HandleFunc("GET /api/transfers/{id}", g.Read(s.getTransfer))
	mux.HandleFunc("POST /api/transfers", g.Write(s.createTransfer))
	mux.HandleFunc("PUT /api/transfers/{id}", g.Write(s.updateTransfer))
	mux.HandleFunc("DELETE /api/transfers/{id}", g.Write(s.deleteTransfer))
	mux.HandleFunc("GET /api/recurring", g.Read(s.listRecurring))
	mux.HandleFunc("POST /api/recurring", g.Write(s.createRecurring))
	mux.HandleFunc("PUT /api/recurring/{id}", g.Write(s.updateRecurring))
	mux.HandleFunc("POST /api/recurring/{id}/active", g.Write(s.setRecurringActive))
	mux.HandleFunc("DELETE /api/recurring/{id}", g.Write(s.deleteRecurring))
	mux.HandleFunc("GET /api/budgets", g.Read(s.listBudgets))
	mux.HandleFunc("POST /api/budgets", g.Write(s.createBudget))
	mux.HandleFunc("PUT /api/budgets/{id}", g.Write(s.updateBudget))
	mux.HandleFunc("DELETE /api/budgets/{id}", g.Write(s.deleteBudget))
	mux.HandleFunc("GET /api/goals", g.Read(s.listGoals))
	mux.HandleFunc("POST /api/goals", g.Write(s.createGoal))
	mux.HandleFunc("PUT /api/goals/{id}", g.Write(s.updateGoal))
	mux.HandleFunc("DELETE /api/goals/{id}", g.Write(s.deleteGoal))
	mux.HandleFunc("POST /api/goals/{id}/archive", g.Write(s.archiveGoal))
	mux.HandleFunc("GET /api/goals/{id}/contributions", g.Read(s.goalContributions))
	mux.HandleFunc("POST /api/goals/{id}/contributions", g.Write(s.contribute))
	mux.HandleFunc("DELETE /api/goals/{id}/contributions/{cid}", g.Write(s.deleteContribution))
	mux.HandleFunc("POST /api/quick/parse", g.Read(s.quickParse))
	mux.HandleFunc("GET /api/transactions/suggest", g.Read(s.suggestNames))
	mux.HandleFunc("POST /api/import/preview", g.Write(s.importPreview))
	mux.HandleFunc("POST /api/import/commit", g.Write(s.importCommit))
	mux.HandleFunc("GET /api/import/batches", g.Read(s.importBatches))
	mux.HandleFunc("DELETE /api/import/batches/{id}", g.Write(s.importUndo))
	mux.HandleFunc("GET /api/dashboard", g.Read(s.dashboard))
	mux.HandleFunc("GET /api/dashboard/slices", g.Read(s.dashboardSlices))
	mux.HandleFunc("GET /api/reports/monthly", g.Read(s.monthlyReport))
	mux.HandleFunc("GET /api/reports/monthly.pdf", g.Read(s.monthlyReportPDF))
}
