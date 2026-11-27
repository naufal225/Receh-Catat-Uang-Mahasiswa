// Package app is the composition root: the one place that knows every concrete
// type and connects them. main and the end-to-end tests both build through it.
package app

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"receh/internal/config"
	deliveryhttp "receh/internal/delivery/http"
	"receh/internal/domain"
	"receh/internal/modules"
	"receh/internal/platform/cache"
	"receh/internal/platform/geoip"
	"receh/internal/platform/keymutex"
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
	// How often scheduled module jobs run; zero means one minute.
	SweepEvery time.Duration
	// Google sign-in provider; nil disables it.
	Google domain.OAuthProvider
	// BillingEnabled false = personal edition (everyone premium).
	BillingEnabled bool
	// Payments is the gateway checkouts go through; nil means no online
	// payment (promo codes and comps still work). Catalog zero = config.ProductCatalog().
	Payments      domain.PaymentProvider
	Catalog       domain.Catalog
	PublicURL     string
	PaymentStatus usecase.IntegrationStatus
	// AppName is the product name modules print (reports, messages).
	AppName string
	// Modules overrides the module list (nil = Modules()).
	Modules []modules.Module
}

type App struct {
	Handler       http.Handler
	Admin         *usecase.AdminService
	Auth          *usecase.AuthService
	Membership    *usecase.MembershipService
	Notifications *usecase.NotificationService
	Billing       *usecase.BillingService
	modules       map[string]modules.Module
	audit         *usecase.AuditRecorder
	janitors      []func(context.Context)
	wg            sync.WaitGroup
}

// Module returns a loaded module by name, or nil.
func (a *App) Module(name string) modules.Module { return a.modules[name] }

func New(p Params) (*App, error) {
	mods := p.Modules
	if mods == nil {
		mods = Modules()
	}
	for _, m := range mods {
		if err := m.Migrate(p.DB); err != nil {
			return nil, fmt.Errorf("module %s: migrate: %w", m.Name(), err)
		}
	}
	if p.AppName == "" {
		p.AppName = "Receh"
	}
	if p.SweepEvery <= 0 {
		p.SweepEvery = time.Minute
	}

	// Sessions and users are read on every authenticated request, so they sit
	// behind a cache. Every service gets the cached wrappers, so the writes that
	// invalidate (revoke, suspend, freeze) always pass through them.
	users := cached.NewUserRepo(mysql.NewUserRepo(p.DB), p.SessionCacheTTL)
	sessions := cached.NewSessionRepo(mysql.NewSessionRepo(p.DB), p.SessionCacheTTL)
	auditRepo := mysql.NewAuditRepo(p.DB)

	audit := usecase.NewAuditRecorder(auditRepo, sessions, geoip.New(p.GeoIPURL), 2)
	// Bumped by every write to a workspace's data; read caches key on it.
	versions := cache.NewVersions()
	// Every write tells all listeners: the version above and the notification worker.
	changes := &usecase.ChangeNotifiers{versions}
	orgs := usecase.NewOrgService(mysql.NewOrgRepo(p.DB), users, audit, p.Clock, p.SessionCacheTTL)
	hooks := usecase.NewUserHooks(orgs)

	auth := usecase.NewAuthService(users, sessions, audit, p.BcryptCost, hooks)
	var janitors []func(context.Context)
	if p.Google != nil {
		auth.EnableGoogle(p.Google)
		janitors = append(janitors, func(ctx context.Context) { auth.OAuthStates().RunJanitor(ctx, time.Minute) })
	}
	membership := usecase.NewMembershipService(p.BillingEnabled, mysql.NewEntitlementRepo(p.DB), mysql.NewPromoRepo(p.DB),
		users, orgs, audit, p.Clock)
	notifications := usecase.NewNotificationService(mysql.NewNotificationRepo(p.DB), users, orgs, p.Clock)
	*changes = append(*changes, notifications)
	if len(p.Catalog.Prices) == 0 {
		p.Catalog = config.ProductCatalog()
	}
	billing := usecase.NewBillingService(usecase.BillingConfig{
		Enabled: p.BillingEnabled, Catalog: p.Catalog, PublicURL: p.PublicURL, Provider: p.Payments, Status: p.PaymentStatus,
		Defs: membership.Defs,
	}, mysql.NewBillingRepo(p.DB), orgs, users, audit, p.Clock)
	notifications.AddSource(billing)

	svc := deliveryhttp.Services{
		Auth:          auth,
		Admin:         usecase.NewAdminService(users, sessions, auditRepo, audit, p.BcryptCost, hooks),
		Membership:    membership,
		Notifications: notifications,
		Orgs:          orgs,
		Billing:       billing,
	}
	server := deliveryhttp.NewServer(svc, deliveryhttp.Options{
		CookieSecure: p.CookieSecure, TrustProxy: p.TrustProxy, StaticDir: p.StaticDir,
	})

	host := &modules.Host{
		DB: p.DB, Clock: p.Clock, Locks: keymutex.New(), Changes: changes, Versions: versions,
		Plans: membership, Membership: membership, Notifications: notifications, Orgs: orgs, Server: server,
		CacheTTL: p.DashboardCacheTTL, SweepEvery: p.SweepEvery, AppName: p.AppName,
	}
	byName := map[string]modules.Module{}
	for _, m := range mods {
		if _, dup := byName[m.Name()]; dup {
			return nil, fmt.Errorf("module %s registered twice", m.Name())
		}
		if err := m.Init(host); err != nil {
			return nil, fmt.Errorf("module %s: init: %w", m.Name(), err)
		}
		byName[m.Name()] = m
	}

	return &App{
		Handler: server.Handler(), Admin: svc.Admin, Auth: auth, Membership: membership,
		Notifications: notifications, Billing: billing, modules: byName, audit: audit,
		janitors: append(append(janitors,
			func(ctx context.Context) { users.Cache().RunJanitor(ctx, time.Minute) },
			func(ctx context.Context) { sessions.Cache().RunJanitor(ctx, time.Minute) },
			auth.Maintain,
			orgs.RunJanitor,
			notifications.RunWorker,
			func(ctx context.Context) { notifications.RunScheduler(ctx, 10*time.Minute) },
			func(ctx context.Context) { billing.RunSweeper(ctx, 10*time.Minute) },
		), host.Jobs()...),
	}, nil
}

// StartBackground launches the long-running goroutines. They end when ctx does.
func (a *App) StartBackground(ctx context.Context) {
	a.wg.Add(len(a.janitors))
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
