// Package payment holds what the gateway adapters (mock, midtrans, xendit)
// share: a small JSON-over-HTTP helper. Each adapter implements
// domain.PaymentProvider and nothing else knows a gateway's API.
package payment

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client makes gateway calls with a bounded time and bounded response size.
var Client = &http.Client{Timeout: 15 * time.Second}

// DoJSON sends body (nil for none) as JSON with HTTP Basic auth (user, empty
// password) and decodes a JSON response into out (nil to ignore it). It
// returns the HTTP status; a non-2xx status is not an error by itself, because
// gateways put useful detail in the body.
func DoJSON(ctx context.Context, c *http.Client, method, url, user string, body, out any) (int, error) {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return 0, err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, rd)
	if err != nil {
		return 0, err
	}
	req.SetBasicAuth(user, "")
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c == nil {
		c = Client
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, err
	}
	if out != nil && len(raw) > 0 {
		if err := json.Unmarshal(raw, out); err != nil {
			return resp.StatusCode, fmt.Errorf("decode response (HTTP %d): %w", resp.StatusCode, err)
		}
	}
	return resp.StatusCode, nil
}
