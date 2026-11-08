// Command api runs the Receh server.
package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"receh/internal/app"
	"receh/internal/config"
	"receh/internal/domain"
	"receh/internal/platform/googleoauth"
	"receh/internal/repository/mysql"
	"receh/internal/usecase"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

// googleProvider returns nil (feature off) unless the client id is configured.
// A nil *Provider must not become a non-nil interface, hence the explicit nil.
func googleProvider(cfg *config.Config) domain.OAuthProvider {
	if cfg.GoogleClientID == "" {
		return nil
	}
	log.Printf("Masuk dengan Google aktif (redirect: %s)", cfg.GoogleRedirectURL)
	return googleoauth.New(cfg.GoogleClientID, cfg.GoogleClientSecret, cfg.GoogleRedirectURL)
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	db, err := mysql.Open(cfg.DBDSN, mysql.Options{MaxOpenConns: cfg.DBMaxOpen})
	if err != nil {
		return err
	}
	defer db.Close()

	staticDir := cfg.StaticDir
	if _, err := os.Stat(staticDir); err != nil {
		staticDir = "" // API-only: the Vite dev server is serving the frontend.
	}
	a := app.New(app.Params{
		DB: db, Clock: usecase.RealClock{Loc: cfg.Location}, BcryptCost: bcrypt.DefaultCost + 2,
		GeoIPURL: cfg.GeoIPURL, CookieSecure: cfg.CookieSecure, TrustProxy: cfg.TrustProxy, StaticDir: staticDir,
		SessionCacheTTL: cfg.SessionTTL, DashboardCacheTTL: cfg.DashboardTTL,
		Google: googleProvider(cfg),
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	generated, err := a.Admin.EnsureAdmin(ctx, cfg.AdminEmail, cfg.AdminPassword)
	if err != nil {
		return err
	}
	if generated != "" {
		log.Printf("=== Admin pertama dibuat. Simpan sekarang, sandi ini tidak ditampilkan lagi ===")
		log.Printf("    email: %s", cfg.AdminEmail)
		log.Printf("    sandi: %s", generated)
	}

	a.StartBackground(ctx, cfg.SweepEvery)

	srv := &http.Server{
		Addr: cfg.Addr, Handler: a.Handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("Receh berjalan di http://localhost%s", cfg.Addr)
		serveErr <- srv.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		stop()
		a.Close()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
	}

	log.Print("Mematikan server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	a.Close()
	return nil
}
