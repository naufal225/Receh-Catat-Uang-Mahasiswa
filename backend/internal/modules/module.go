// Package modules defines how a feature module plugs into the core. A module
// owns its tables, routes, plan limits, notifications and background jobs; the
// core (accounts, sessions, admin, plans, notifications) never imports one.
//
// To remove a module, delete its directory and its line in app/modules.go.
package modules

import (
	"context"
	"database/sql"
	"time"

	core "receh/internal/delivery/http"
	"receh/internal/domain"
	"receh/internal/kit"
	"receh/internal/platform/cache"
	"receh/internal/platform/keymutex"
	"receh/internal/usecase"
)

// Module is one pluggable feature.
type Module interface {
	// Name is a short, stable id ("finance"); also the prefix of its limit keys.
	Name() string
	// Migrate creates or updates the module's tables. It runs after the core
	// schema, on every start, and must be idempotent.
	Migrate(db *sql.DB) error
	// Init builds the module's services and registers them with the host.
	Init(h *Host) error
}

// Host is what the core offers a module during Init.
type Host struct {
	DB    *sql.DB
	Clock domain.Clock
	// Locks is shared by every path that must serialise per row or per
	// workspace (kit.ScopeLockKey), across modules.
	Locks *keymutex.Map
	// Changes must be told (with the org id) after every write to a workspace's
	// data: it bumps Versions (cache keys) and wakes the notification worker.
	Changes  domain.ChangeNotifier
	Versions *cache.Versions
	// Plans answers plan limits registered with Membership.AddLimits.
	Plans kit.PlanLimits

	Membership    *usecase.MembershipService
	Notifications *usecase.NotificationService
	// Orgs owns workspaces. Module data belongs to a workspace: key your tables
	// by org_id and read the id of the request with core.OrgFrom. Use
	// Orgs.OnOrgCreated to seed a new workspace.
	Orgs   *usecase.OrgService
	Server *core.Server

	// AppName is the product name, for reports and messages.
	AppName string
	// CacheTTL is the configured read-cache lifetime (0 disables caching).
	CacheTTL time.Duration
	// SweepEvery is how often scheduled jobs (e.g. recurring entries) run.
	SweepEvery time.Duration

	jobs []func(context.Context)
}

// Background registers a long-running job; it starts with the app and must
// return when ctx ends.
func (h *Host) Background(job func(ctx context.Context)) { h.jobs = append(h.jobs, job) }

// Jobs returns the registered background jobs (used by the app).
func (h *Host) Jobs() []func(context.Context) { return h.jobs }
