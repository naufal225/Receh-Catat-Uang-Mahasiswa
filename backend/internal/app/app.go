// Package app is the composition root: the one place that knows every concrete
// type and connects them. main and the end-to-end tests both build through it.
package app

import (
	"context"
	"database/sql"
	"net/http"
	"sync"
	"time"

	deliveryhttp "receh/internal/delivery/http"
	"receh/internal/domain"
	"receh/internal/platform/cache"
	"receh/internal/platform/geoip"
	"receh/internal/platform/keymutex"
	"receh/internal/platform/pdf"
	"receh/internal/repository/cached"
	"receh/internal/repository/mysql"
	"receh/internal/usecase"
)

type Params struct {
	DB           *sql.DB
	Clock        domain.Clock
	BcryptCost   int
	GeoIPURL     string
	CookieSecure bool
	TrustProxy   bool
	StaticDir    string
	// Zero disables the cache.
	SessionCacheTTL   time.Duration
	DashboardCacheTTL time.Duration
	// Google sign-in provider; nil disables it.
	Google domain.OAuthProvider
	// BillingEnabled false = personal edition (everyone premium).
	BillingEnabled bool
}

type App struct {
	Handler   http.Handler
	Admin     *usecase.AdminService
	Recurring *usecase.RecurringService
	Auth      *usecase.AuthService
	Txs       *usecase.TransactionService
	audit     *usecase.AuditRecorder
	janitors  []func(context.Context)
	wg        sync.WaitGroup
}

func New(p Params) *App {
	// Sessions and users are read on every authenticated request, so they sit
	// behind a cache. Every service gets the cached wrappers, so the writes that
	// invalidate (revoke, suspend, freeze) always pass through them.
	users := cached.NewUserRepo(mysql.NewUserRepo(p.DB), p.SessionCacheTTL)
	sessions := cached.NewSessionRepo(mysql.NewSessionRepo(p.DB), p.SessionCacheTTL)
	wallets := mysql.NewWalletRepo(p.DB)
	categories := mysql.NewCategoryRepo(p.DB)
	txs := mysql.NewTransactionRepo(p.DB)
	transfers := mysql.NewTransferRepo(p.DB)
	recurring := mysql.NewRecurringRepo(p.DB)
	auditRepo := mysql.NewAuditRepo(p.DB)

	audit := usecase.NewAuditRecorder(auditRepo, sessions, geoip.New(p.GeoIPURL), 2)
	// One lock table shared by every path that moves money.
	locks := keymutex.New()
	// Bumped by every write to a user's finances; the dashboard cache keys on it.
	versions := cache.NewVersions()

	auth := usecase.NewAuthService(users, sessions, audit, p.BcryptCost)
	var janitors []func(context.Context)
	if p.Google != nil {
		auth.EnableGoogle(p.Google)
		janitors = append(janitors, func(ctx context.Context) { auth.OAuthStates().RunJanitor(ctx, time.Minute) })
	}

	membership := usecase.NewMembershipService(p.BillingEnabled, mysql.NewEntitlementRepo(p.DB), mysql.NewPromoRepo(p.DB),
		users, wallets, audit, p.Clock)

	dashboard := usecase.NewDashboardService(txs, wallets, recurring, p.Clock, versions, p.DashboardCacheTTL)
	budgets := usecase.NewBudgetService(mysql.NewBudgetRepo(p.DB), categories, txs, p.Clock, locks, membership, versions)
	dashboard.WithBudgets(budgets)
	membership.AddUsageCounter(budgets.Count)
	goals := usecase.NewGoalService(mysql.NewGoalRepo(p.DB), wallets, p.Clock, locks, membership, versions)
	dashboard.WithGoals(goals)
	membership.AddUsageCounter(goals.Count)
	svc := deliveryhttp.Services{
		Auth:       auth,
		Wallets:    usecase.NewWalletService(wallets, recurring, locks, versions, membership),
		Categories: usecase.NewCategoryService(categories, versions),
		Txs:        usecase.NewTransactionService(txs, wallets, categories, p.Clock, locks, versions),
		Transfers:  usecase.NewTransferService(transfers, wallets, categories, p.Clock, locks, versions),
		Recurring:  usecase.NewRecurringService(recurring, wallets, categories, p.Clock, locks, 4, versions),
		Dashboard:  dashboard,
		Reports:    usecase.NewReportService(txs, pdf.ReportRenderer{}, p.Clock),
		Admin:      usecase.NewAdminService(users, sessions, auditRepo, audit, p.BcryptCost),
		Membership: membership,
		Budgets:    budgets,
		Goals:      goals,
	}
	server := deliveryhttp.NewServer(svc, deliveryhttp.Options{
		CookieSecure: p.CookieSecure, TrustProxy: p.TrustProxy, StaticDir: p.StaticDir,
	})
	return &App{
		Handler: server.Handler(), Admin: svc.Admin, Recurring: svc.Recurring,
		Auth: svc.Auth, Txs: svc.Txs, audit: audit,
		janitors: append(janitors,
			func(ctx context.Context) { users.Cache().RunJanitor(ctx, time.Minute) },
			func(ctx context.Context) { sessions.Cache().RunJanitor(ctx, time.Minute) },
			func(ctx context.Context) { dashboard.Cache().RunJanitor(ctx, time.Minute) },
		),
	}
}

// StartBackground launches the long-running goroutines. They end when ctx does.
func (a *App) StartBackground(ctx context.Context, sweepEvery time.Duration) {
	a.wg.Add(2 + len(a.janitors))
	go func() {
		defer a.wg.Done()
		a.Recurring.RunScheduler(ctx, sweepEvery)
	}()
	go func() {
		defer a.wg.Done()
		a.Auth.Maintain(ctx)
	}()
	for _, run := range a.janitors {
		go func() {
			defer a.wg.Done()
			run(ctx)
		}()
	}
}

// Close waits for the background goroutines and pending audit lookups.
func (a *App) Close() {
	a.wg.Wait()
	a.audit.Close()
}
