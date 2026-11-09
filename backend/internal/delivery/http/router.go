// Package http is the delivery layer: it translates HTTP requests into use case
// calls and results into JSON. No business rule lives here.
package http

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"receh/internal/usecase"
)

// Services groups what the handlers call.
type Services struct {
	Auth          *usecase.AuthService
	Wallets       *usecase.WalletService
	Categories    *usecase.CategoryService
	Txs           *usecase.TransactionService
	Transfers     *usecase.TransferService
	Recurring     *usecase.RecurringService
	Dashboard     *usecase.DashboardService
	Reports       *usecase.ReportService
	Admin         *usecase.AdminService
	Membership    *usecase.MembershipService
	Budgets       *usecase.BudgetService
	Goals         *usecase.GoalService
	Notifications *usecase.NotificationService
	Entries       *usecase.EntryService
}

type Options struct {
	CookieSecure bool
	TrustProxy   bool
	StaticDir    string
}

type Server struct {
	svc  Services
	opts Options
}

func NewServer(svc Services, opts Options) *Server { return &Server{svc: svc, opts: opts} }

// Handler builds the route table and wraps it in the middleware chain.
//
// Route guards: anyone(...) needs any session, userRead(...) needs a user
// account, userWrite(...) additionally refuses frozen accounts, admin(...) needs
// an admin account.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/auth/register", s.register)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("GET /api/auth/providers", s.providers)
	mux.HandleFunc("GET /api/auth/google/start", s.googleStart)
	mux.HandleFunc("GET /api/auth/google/callback", s.googleCallback)
	mux.HandleFunc("POST /api/auth/logout", s.anyone(s.logout))
	mux.HandleFunc("GET /api/auth/me", s.anyone(s.me))
	mux.HandleFunc("POST /api/auth/password", s.anyone(s.changePassword))

	mux.HandleFunc("GET /api/sessions", s.anyone(s.listSessions))
	mux.HandleFunc("DELETE /api/sessions/{id}", s.anyone(s.revokeSession))
	mux.HandleFunc("DELETE /api/sessions", s.anyone(s.revokeOtherSessions))

	mux.HandleFunc("GET /api/wallets", s.userRead(s.listWallets))
	mux.HandleFunc("POST /api/wallets", s.userWrite(s.createWallet))
	mux.HandleFunc("PUT /api/wallets/{id}", s.userWrite(s.updateWallet))
	mux.HandleFunc("DELETE /api/wallets/{id}", s.userWrite(s.deleteWallet))

	mux.HandleFunc("GET /api/categories", s.userRead(s.listCategories))
	mux.HandleFunc("POST /api/categories", s.userWrite(s.createCategory))
	mux.HandleFunc("PUT /api/categories/{id}", s.userWrite(s.updateCategory))
	mux.HandleFunc("DELETE /api/categories/{id}", s.userWrite(s.deleteCategory))

	mux.HandleFunc("GET /api/transactions", s.userRead(s.listTransactions))
	mux.HandleFunc("POST /api/transactions", s.userWrite(s.createTransaction))
	mux.HandleFunc("PUT /api/transactions/{id}", s.userWrite(s.updateTransaction))
	mux.HandleFunc("DELETE /api/transactions/{id}", s.userWrite(s.deleteTransaction))

	mux.HandleFunc("GET /api/transfers/last-fee", s.userRead(s.transferFee))
	mux.HandleFunc("GET /api/transfers/{id}", s.userRead(s.getTransfer))
	mux.HandleFunc("POST /api/transfers", s.userWrite(s.createTransfer))
	mux.HandleFunc("PUT /api/transfers/{id}", s.userWrite(s.updateTransfer))
	mux.HandleFunc("DELETE /api/transfers/{id}", s.userWrite(s.deleteTransfer))

	mux.HandleFunc("GET /api/recurring", s.userRead(s.listRecurring))
	mux.HandleFunc("POST /api/recurring", s.userWrite(s.createRecurring))
	mux.HandleFunc("PUT /api/recurring/{id}", s.userWrite(s.updateRecurring))
	mux.HandleFunc("POST /api/recurring/{id}/active", s.userWrite(s.setRecurringActive))
	mux.HandleFunc("DELETE /api/recurring/{id}", s.userWrite(s.deleteRecurring))

	mux.HandleFunc("GET /api/budgets", s.userRead(s.listBudgets))
	mux.HandleFunc("POST /api/budgets", s.userWrite(s.createBudget))
	mux.HandleFunc("PUT /api/budgets/{id}", s.userWrite(s.updateBudget))
	mux.HandleFunc("DELETE /api/budgets/{id}", s.userWrite(s.deleteBudget))

	mux.HandleFunc("GET /api/goals", s.userRead(s.listGoals))
	mux.HandleFunc("POST /api/goals", s.userWrite(s.createGoal))
	mux.HandleFunc("PUT /api/goals/{id}", s.userWrite(s.updateGoal))
	mux.HandleFunc("DELETE /api/goals/{id}", s.userWrite(s.deleteGoal))
	mux.HandleFunc("POST /api/goals/{id}/archive", s.userWrite(s.archiveGoal))
	mux.HandleFunc("GET /api/goals/{id}/contributions", s.userRead(s.goalContributions))
	mux.HandleFunc("POST /api/goals/{id}/contributions", s.userWrite(s.contribute))
	mux.HandleFunc("DELETE /api/goals/{id}/contributions/{cid}", s.userWrite(s.deleteContribution))

	mux.HandleFunc("GET /api/notifications", s.userRead(s.inbox))
	mux.HandleFunc("GET /api/notifications/unread", s.userRead(s.unreadCount))
	mux.HandleFunc("POST /api/notifications/read", s.userRead(s.markRead))
	mux.HandleFunc("GET /api/notifications/prefs", s.userRead(s.notificationPrefs))
	mux.HandleFunc("PUT /api/notifications/prefs", s.userWrite(s.saveNotificationPrefs))

	mux.HandleFunc("POST /api/quick/parse", s.userRead(s.quickParse))
	mux.HandleFunc("GET /api/transactions/suggest", s.userRead(s.suggestNames))
	mux.HandleFunc("POST /api/import/preview", s.userWrite(s.importPreview))
	mux.HandleFunc("POST /api/import/commit", s.userWrite(s.importCommit))
	mux.HandleFunc("GET /api/import/batches", s.userRead(s.importBatches))
	mux.HandleFunc("DELETE /api/import/batches/{id}", s.userWrite(s.importUndo))

	mux.HandleFunc("GET /api/membership", s.userRead(s.membership))
	mux.HandleFunc("POST /api/membership/redeem", s.userWrite(s.redeemCode))

	mux.HandleFunc("GET /api/dashboard", s.userRead(s.dashboard))
	mux.HandleFunc("GET /api/dashboard/slices", s.userRead(s.dashboardSlices))

	mux.HandleFunc("GET /api/reports/monthly", s.userRead(s.monthlyReport))
	mux.HandleFunc("GET /api/reports/monthly.pdf", s.userRead(s.monthlyReportPDF))

	mux.HandleFunc("GET /api/admin/users", s.admin(s.adminListUsers))
	mux.HandleFunc("POST /api/admin/users", s.admin(s.adminCreateUser))
	mux.HandleFunc("POST /api/admin/users/{id}/status", s.admin(s.adminSetStatus))
	mux.HandleFunc("GET /api/admin/audit-logs", s.admin(s.adminAuditLogs))
	mux.HandleFunc("GET /api/admin/users/{id}/premium", s.admin(s.adminUserPremium))
	mux.HandleFunc("POST /api/admin/users/{id}/premium", s.admin(s.adminGrantPremium))
	mux.HandleFunc("DELETE /api/admin/premium/{id}", s.admin(s.adminRevokePremium))
	mux.HandleFunc("GET /api/admin/promo-codes", s.admin(s.adminListPromos))
	mux.HandleFunc("POST /api/admin/promo-codes", s.admin(s.adminCreatePromo))
	mux.HandleFunc("POST /api/admin/promo-codes/{id}/active", s.admin(s.adminSetPromoActive))

	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusNotFound, errorBody{Code: "NOT_FOUND", Message: "Endpoint tidak ditemukan."})
	})
	if s.opts.StaticDir != "" {
		mux.Handle("/", spaHandler(s.opts.StaticDir))
	}
	return recoverPanic(logRequests(securityHeaders(sameOrigin(mux))))
}

// spaHandler serves the built frontend, answering unknown paths with index.html
// so client-side routes survive a page reload.
func spaHandler(dir string) http.Handler {
	files := http.FileServer(http.Dir(dir))
	index := filepath.Join(dir, "index.html")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		clean := filepath.Join(dir, filepath.FromSlash(filepath.Clean("/"+r.URL.Path)))
		if info, err := os.Stat(clean); err == nil && !info.IsDir() {
			if strings.HasPrefix(r.URL.Path, "/assets/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			}
			files.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFile(w, r, index)
	})
}
