// Package geoip resolves an IP address to a city and country. Private addresses
// never leave the machine, results are cached behind a sync.Mutex, and a failed
// lookup degrades to "Tidak diketahui" rather than affecting a login.
package geoip

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	LocalNetwork = "Jaringan lokal"
	Unknown      = "Tidak diketahui"
	Disabled     = "Pelacakan lokasi nonaktif"
)

// Locator calls a JSON geolocation endpoint. BaseURL + ip must return
// {"success": bool, "city": string, "region": string, "country": string}.
type Locator struct {
	baseURL string
	client  *http.Client

	mu    sync.Mutex
	cache map[string]string
}

// New returns a Locator. An empty baseURL turns remote lookups off.
func New(baseURL string) *Locator {
	return &Locator{
		baseURL: baseURL,
		client:  &http.Client{Timeout: 4 * time.Second},
		cache:   make(map[string]string),
	}
}

func (l *Locator) Locate(ctx context.Context, ip string) string {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return Unknown
	}
	if parsed.IsLoopback() || parsed.IsPrivate() || parsed.IsLinkLocalUnicast() {
		return LocalNetwork
	}
	if l.baseURL == "" {
		return Disabled
	}

	l.mu.Lock()
	cached, ok := l.cache[ip]
	l.mu.Unlock()
	if ok {
		return cached
	}

	place, err := l.fetch(ctx, ip)
	if err != nil {
		return Unknown // Not cached, so the next login from this IP tries again.
	}
	l.mu.Lock()
	if len(l.cache) > 5000 { // Plenty for a campus app; simple bound on memory.
		l.cache = make(map[string]string)
	}
	l.cache[ip] = place
	l.mu.Unlock()
	return place
}

func (l *Locator) fetch(ctx context.Context, ip string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, l.baseURL+ip, nil)
	if err != nil {
		return "", err
	}
	resp, err := l.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("geoip: status %d", resp.StatusCode)
	}
	var body struct {
		Success *bool  `json:"success"`
		City    string `json:"city"`
		Region  string `json:"region"`
		Country string `json:"country"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", err
	}
	if body.Success != nil && !*body.Success {
		return "", fmt.Errorf("geoip: lookup refused for %s", ip)
	}
	var parts []string
	for _, p := range []string{body.City, body.Region, body.Country} {
		if p != "" && !containsFold(parts, p) {
			parts = append(parts, p)
		}
	}
	if len(parts) == 0 {
		return Unknown, nil
	}
	return strings.Join(parts, ", "), nil
}

func containsFold(list []string, s string) bool {
	for _, v := range list {
		if strings.EqualFold(v, s) {
			return true
		}
	}
	return false
}
