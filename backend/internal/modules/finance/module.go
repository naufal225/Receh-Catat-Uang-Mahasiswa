// Package finance is the example module of the starter kit: wallets,
// transactions, transfers, budgets, goals, recurring entries, reports and
// CSV import. It shows every extension point a module can use.
package finance

import (
	"context"
	"database/sql"
	"time"

	"receh/internal/domain"
	"receh/internal/modules"
	"receh/internal/modules/finance/api"
	"receh/internal/modules/finance/report"
	"receh/internal/modules/finance/service"
	"receh/internal/modules/finance/store"
)

type Module struct {
	// Services, available after Init (tests and admin tools use them).
	Services api.Services
}

func New() *Module { return &Module{} }

func (m *Module) Name() string { return "finance" }

func (m *Module) Migrate(db *sql.DB) error { return store.Migrate(db) }

func lbl(id, en string) map[string]string { return map[string]string{"id": id, "en": en} }

// Limits are the finance module's plan limits.
var Limits = []domain.LimitDef{
	{Key: service.LimitWallets, Label: lbl("Dompet aktif", "Active wallets"), Free: 3, Premium: domain.Unlimited},
	{Key: service.LimitBudgets, Label: lbl("Anggaran kategori", "Category budgets"), Free: 3, Premium: domain.Unlimited},
	{Key: service.LimitGoals, Label: lbl("Target tabungan aktif", "Active savings goals"), Free: 1, Premium: domain.Unlimited},
	{Key: service.LimitCSVImport, Label: lbl("Impor mutasi CSV", "CSV statement import"), Feature: true, Free: 0, Premium: 1},
}

func (m *Module) Init(h *modules.Host) error {
	wallets := store.NewWalletRepo(h.DB)
	categories := store.NewCategoryRepo(h.DB)
	txs := store.NewTransactionRepo(h.DB)
	recurring := store.NewRecurringRepo(h.DB)

	dashboard := service.NewDashboardService(txs, wallets, recurring, h.Clock, h.Versions, h.CacheTTL)
	budgets := service.NewBudgetService(store.NewBudgetRepo(h.DB), categories, txs, h.Clock, h.Locks, h.Plans, h.Changes)
	goals := service.NewGoalService(store.NewGoalRepo(h.DB), wallets, h.Clock, h.Locks, h.Plans, h.Changes)
	dashboard.WithBudgets(budgets)
	dashboard.WithGoals(goals)
	rec := service.NewRecurringService(recurring, wallets, categories, h.Clock, h.Locks, 4, h.Changes)

	m.Services = api.Services{
		Wallets:    service.NewWalletService(wallets, recurring, h.Locks, h.Changes, h.Plans),
		Categories: service.NewCategoryService(categories, h.Changes),
		Txs:        service.NewTransactionService(txs, wallets, categories, h.Clock, h.Locks, h.Changes),
		Transfers:  service.NewTransferService(store.NewTransferRepo(h.DB), wallets, categories, h.Clock, h.Locks, h.Changes),
		Recurring:  rec,
		Dashboard:  dashboard,
		Reports:    service.NewReportService(txs, report.ReportRenderer{AppName: h.AppName}, h.Clock),
		Budgets:    budgets,
		Goals:      goals,
		Entries:    service.NewEntryService(store.NewEntryRepo(h.DB), wallets, categories, h.Clock, h.Locks, h.Plans, h.Changes),
	}

	h.Membership.AddLimits(Limits...)
	h.Membership.AddUsageCounter(func(ctx context.Context, orgID int64, u domain.Usage) error {
		ws, err := wallets.List(ctx, orgID, false)
		u[service.LimitWallets] = len(ws)
		return err
	})
	h.Membership.AddUsageCounter(budgets.Count)
	h.Membership.AddUsageCounter(goals.Count)

	h.Notifications.AddSource(service.NewNotifications(wallets, recurring, txs, budgets, goals, h.Orgs, h.Clock, h.AppName))

	h.Orgs.OnOrgCreated(func(ctx context.Context, orgID int64) error {
		return categories.CreateDefaults(ctx, orgID, service.DefaultCategories())
	})

	h.Server.Mount(api.New(m.Services).Mount)

	h.Background(func(ctx context.Context) { rec.RunScheduler(ctx, h.SweepEvery) })
	h.Background(func(ctx context.Context) { dashboard.Cache().RunJanitor(ctx, time.Minute) })
	return nil
}
