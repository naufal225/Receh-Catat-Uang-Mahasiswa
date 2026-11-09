// Package config reads settings from the environment and an optional .env file.
package config

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

type Config struct {
	Addr          string
	DBDSN         string
	DBMaxOpen     int
	SessionTTL    time.Duration
	DashboardTTL  time.Duration
	StaticDir     string
	Location      *time.Location
	CookieSecure  bool
	TrustProxy    bool
	GeoIPURL      string
	AdminEmail    string
	AdminPassword string
	SweepEvery    time.Duration
	// BillingEnabled false is the personal edition: everyone is premium.
	BillingEnabled bool
	// Google sign-in; disabled when GoogleClientID is empty.
	GoogleClientID     string
	GoogleClientSecret string
	GoogleRedirectURL  string
}

// Load reads .env if present (real environment variables win) and applies defaults.
func Load() (*Config, error) {
	loadDotEnv(".env")

	tz := env("APP_TIMEZONE", "Asia/Jakarta")
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// Windows may lack tzdata. Asia/Jakarta is a fixed UTC+7 with no DST.
		if tz != "Asia/Jakarta" {
			return nil, fmt.Errorf("APP_TIMEZONE %q: %w", tz, err)
		}
		loc = time.FixedZone("WIB", 7*3600)
	}
	dsn, err := databaseDSN()
	if err != nil {
		return nil, err
	}
	maxOpen, err := strconv.Atoi(env("DB_MAX_OPEN_CONNS", "20"))
	if err != nil || maxOpen <= 0 {
		return nil, fmt.Errorf("DB_MAX_OPEN_CONNS must be a positive number")
	}
	sessionTTL, err := cacheTTL("SESSION_CACHE_TTL", "30s")
	if err != nil {
		return nil, err
	}
	dashTTL, err := cacheTTL("DASHBOARD_CACHE_TTL", "60s")
	if err != nil {
		return nil, err
	}
	googleID := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID"))
	googleSecret := strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_SECRET"))
	googleRedirect := strings.TrimSpace(os.Getenv("GOOGLE_REDIRECT_URL"))
	if googleID != "" && (googleSecret == "" || googleRedirect == "") {
		return nil, fmt.Errorf("GOOGLE_CLIENT_ID is set, so GOOGLE_CLIENT_SECRET and GOOGLE_REDIRECT_URL are required too")
	}
	sweep, err := time.ParseDuration(env("RECURRING_SWEEP_EVERY", "1h"))
	if err != nil || sweep <= 0 {
		return nil, fmt.Errorf("RECURRING_SWEEP_EVERY must be a positive duration such as 1h")
	}
	return &Config{
		Addr:          env("ADDR", ":8080"),
		DBDSN:         dsn,
		DBMaxOpen:     maxOpen,
		SessionTTL:    sessionTTL,
		DashboardTTL:  dashTTL,
		StaticDir:     env("STATIC_DIR", "../frontend/dist"),
		Location:      loc,
		CookieSecure:  env("COOKIE_SECURE", "false") == "true",
		TrustProxy:    env("TRUST_PROXY", "false") == "true",
		GeoIPURL:      env("GEOIP_URL", "https://ipwho.is/"),
		AdminEmail:    env("ADMIN_EMAIL", "admin@receh.local"),
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
		SweepEvery:    sweep,
		BillingEnabled:     env("BILLING_ENABLED", "false") == "true",
		GoogleClientID:     googleID,
		GoogleClientSecret: googleSecret,
		GoogleRedirectURL:  googleRedirect,
	}, nil
}

// databaseDSN builds the MySQL DSN from DB_* variables. DB_DSN, when set, wins
// and is used verbatim (go-sql-driver format: user:pass@tcp(host:port)/name).
func databaseDSN() (string, error) {
	if dsn := os.Getenv("DB_DSN"); dsn != "" {
		return dsn, nil
	}
	name := env("DB_NAME", "receh")
	if name == "" {
		return "", fmt.Errorf("DB_NAME must not be empty")
	}
	cfg := mysql.NewConfig()
	cfg.User = env("DB_USER", "root")
	cfg.Passwd = os.Getenv("DB_PASSWORD")
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(env("DB_HOST", "127.0.0.1"), env("DB_PORT", "3306"))
	cfg.DBName = name
	return cfg.FormatDSN(), nil
}

// cacheTTL reads a duration where 0 switches that cache off.
func cacheTTL(key, fallback string) (time.Duration, error) {
	d, err := time.ParseDuration(env(key, fallback))
	if err != nil || d < 0 {
		return 0, fmt.Errorf("%s must be a duration such as 30s, or 0 to disable", key)
	}
	return d, nil
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return fallback
}

func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if _, exists := os.LookupEnv(key); !exists {
			os.Setenv(key, strings.Trim(strings.TrimSpace(value), `"'`))
		}
	}
}
