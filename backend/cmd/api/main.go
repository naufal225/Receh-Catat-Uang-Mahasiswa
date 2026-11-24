// Command api runs the Receh server.
package main

import (
	"context"
	"errors"
	"fmt"
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
	"receh/internal/platform/payment/midtrans"
	mockpay "receh/internal/platform/payment/mock"
	"receh/internal/platform/payment/xendit"
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

// paymentProvider builds the gateway chosen by BILLING_PROVIDER, plus the
// yes/no status the admin page shows (never the keys themselves).
func paymentProvider(cfg *config.Config) (domain.PaymentProvider, usecase.IntegrationStatus, error) {
	st := usecase.IntegrationStatus{
		Provider:   cfg.BillingProvider,
		ReturnURL:  cfg.PublicBaseURL + "/app/langganan?status=return",
		WebhookURL: cfg.PublicBaseURL + "/api/billing/webhook/" + cfg.BillingProvider,
	}
	if !cfg.BillingEnabled {
		return nil, usecase.IntegrationStatus{Provider: "off", Keys: []usecase.KeyStatus{}}, nil
	}
	mode := func(prod bool) string {
		if prod {
			return "production"
		}
		return "sandbox"
	}
	switch cfg.BillingProvider {
	case "mock":
		if cfg.CookieSecure && !cfg.MockAllowProd {
			return nil, st, fmt.Errorf("BILLING_PROVIDER=mock is a fake gateway and COOKIE_SECURE=true suggests production; " +
				"pick midtrans or xendit, or set MOCK_ALLOW_PROD=true if you really mean it")
		}
		log.Print("PERINGATAN: gateway pembayaran TIRUAN (mock) aktif; tidak ada uang sungguhan yang diproses")
		st.Mode, st.Keys, st.WebhookURL = "local", []usecase.KeyStatus{}, ""
		return mockpay.New(cfg.PublicBaseURL), st, nil
	case "midtrans":
		st.Mode = mode(cfg.MidtransProduction)
		st.Keys = []usecase.KeyStatus{{Name: "MIDTRANS_SERVER_KEY", Set: cfg.MidtransServerKey != ""}}
		return midtrans.New(cfg.MidtransServerKey, cfg.MidtransProduction), st, nil
	case "xendit":
		st.Mode = "sandbox/production sesuai kunci"
		st.Keys = []usecase.KeyStatus{
			{Name: "XENDIT_SECRET_KEY", Set: cfg.XenditSecretKey != ""},
			{Name: "XENDIT_CALLBACK_TOKEN", Set: cfg.XenditCallbackToken != ""},
		}
		return xendit.New(cfg.XenditSecretKey, cfg.XenditCallbackToken), st, nil
	}
	return nil, st, fmt.Errorf("unknown BILLING_PROVIDER %q", cfg.BillingProvider)
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
	pay, payStatus, err := paymentProvider(cfg)
	if err != nil {
		return err
	}
	a, err := app.New(app.Params{
		Payments: pay, Catalog: config.ProductCatalog(), PublicURL: cfg.PublicBaseURL, PaymentStatus: payStatus,
		DB: db, Clock: usecase.RealClock{Loc: cfg.Location}, BcryptCost: bcrypt.DefaultCost + 2,
		GeoIPURL: cfg.GeoIPURL, CookieSecure: cfg.CookieSecure, TrustProxy: cfg.TrustProxy, StaticDir: staticDir,
		SessionCacheTTL: cfg.SessionTTL, DashboardCacheTTL: cfg.DashboardTTL,
		Google: googleProvider(cfg), BillingEnabled: cfg.BillingEnabled, SweepEvery: cfg.SweepEvery, AppName: cfg.AppName,
	})
	if err != nil {
		return err
	}

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

	a.StartBackground(ctx)

	srv := &http.Server{
		Addr: cfg.Addr, Handler: a.Handler,
		ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
		WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second,
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("%s berjalan di http://localhost%s", cfg.AppName, cfg.Addr)
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
