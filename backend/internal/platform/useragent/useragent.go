// Package useragent turns a User-Agent header into a label a person recognises,
// such as "Chrome di Windows".
package useragent

import "strings"

// Describe returns "<browser> di <os>", or a short fallback for unknown clients.
func Describe(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return "Perangkat tidak dikenal"
	}
	browser, os := browserOf(ua), osOf(ua)
	switch {
	case browser != "" && os != "":
		return browser + " di " + os
	case browser != "":
		return browser
	case os != "":
		return "Peramban di " + os
	}
	if len(ua) > 40 {
		return ua[:40] + "…"
	}
	return ua
}

// Order matters: Edge and Opera also advertise Chrome, and Chrome advertises Safari.
func browserOf(ua string) string {
	checks := []struct{ token, name string }{
		{"Edg/", "Edge"},
		{"OPR/", "Opera"},
		{"SamsungBrowser/", "Samsung Internet"},
		{"Firefox/", "Firefox"},
		{"FxiOS/", "Firefox"},
		{"CriOS/", "Chrome"},
		{"Chrome/", "Chrome"},
		{"Safari/", "Safari"},
		{"curl/", "curl"},
		{"PowerShell", "PowerShell"},
		{"Go-http-client", "Go client"},
	}
	for _, c := range checks {
		if strings.Contains(ua, c.token) {
			return c.name
		}
	}
	return ""
}

// Android and iOS are checked first because their agents also mention Linux and Mac OS X.
func osOf(ua string) string {
	switch {
	case strings.Contains(ua, "Android"):
		return "Android"
	case strings.Contains(ua, "iPhone"):
		return "iPhone"
	case strings.Contains(ua, "iPad"):
		return "iPad"
	case strings.Contains(ua, "Windows"):
		return "Windows"
	case strings.Contains(ua, "Mac OS X"):
		return "macOS"
	case strings.Contains(ua, "CrOS"):
		return "ChromeOS"
	case strings.Contains(ua, "Linux"):
		return "Linux"
	}
	return ""
}
