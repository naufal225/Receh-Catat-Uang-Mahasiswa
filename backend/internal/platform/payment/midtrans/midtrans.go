// Package midtrans talks to Midtrans (Snap for checkout, Core API for status).
// It uses the REST API directly, no SDK.
package midtrans

import (
	"context"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/payment"
)

type Provider struct {
	serverKey string
	snapURL   string // e.g. https://app.sandbox.midtrans.com
	apiURL    string // e.g. https://api.sandbox.midtrans.com
	http      *http.Client
}

// New builds the adapter. production false uses the sandbox hosts.
func New(serverKey string, production bool) *Provider {
	p := &Provider{serverKey: serverKey, http: payment.Client}
	if production {
		p.snapURL, p.apiURL = "https://app.midtrans.com", "https://api.midtrans.com"
	} else {
		p.snapURL, p.apiURL = "https://app.sandbox.midtrans.com", "https://api.sandbox.midtrans.com"
	}
	return p
}

// WithBaseURLs points the adapter at other hosts (tests use a local server).
func (p *Provider) WithBaseURLs(snap, api string) *Provider {
	p.snapURL, p.apiURL = strings.TrimRight(snap, "/"), strings.TrimRight(api, "/")
	return p
}

func (p *Provider) Name() string { return "midtrans" }

func (p *Provider) CreateCheckout(ctx context.Context, req domain.CheckoutRequest) (domain.Checkout, error) {
	minutes := max(int(time.Until(req.ExpiresAt).Minutes()), 1)
	body := map[string]any{
		"transaction_details": map[string]any{"order_id": req.ExternalID, "gross_amount": req.Amount},
		"customer_details":    map[string]any{"first_name": req.PayerName, "email": req.PayerEmail},
		"item_details":        []map[string]any{{"id": "plan", "price": req.Amount, "quantity": 1, "name": truncate(req.Description, 50)}},
		"expiry":              map[string]any{"unit": "minutes", "duration": minutes},
		"callbacks":           map[string]any{"finish": req.ReturnURL},
	}
	var out struct {
		Token         string   `json:"token"`
		RedirectURL   string   `json:"redirect_url"`
		ErrorMessages []string `json:"error_messages"`
	}
	status, err := payment.DoJSON(ctx, p.http, http.MethodPost, p.snapURL+"/snap/v1/transactions", p.serverKey, body, &out)
	if err != nil {
		return domain.Checkout{}, fmt.Errorf("midtrans: create transaction: %w", err)
	}
	if status/100 != 2 || out.RedirectURL == "" {
		return domain.Checkout{}, fmt.Errorf("midtrans: create transaction: HTTP %d %s", status, strings.Join(out.ErrorMessages, "; "))
	}
	return domain.Checkout{ProviderRef: out.Token, URL: out.RedirectURL}, nil
}

// transaction is the part of Midtrans status and notification bodies we use.
type transaction struct {
	StatusCode        string `json:"status_code"`
	StatusMessage     string `json:"status_message"`
	TransactionID     string `json:"transaction_id"`
	OrderID           string `json:"order_id"`
	GrossAmount       string `json:"gross_amount"`
	TransactionStatus string `json:"transaction_status"`
	FraudStatus       string `json:"fraud_status"`
	SignatureKey      string `json:"signature_key"`
}

func (p *Provider) Cancel(ctx context.Context, externalID, _ string) error {
	var out transaction
	status, err := payment.DoJSON(ctx, p.http, http.MethodPost, p.apiURL+"/v2/"+url.PathEscape(externalID)+"/cancel", p.serverKey, nil, &out)
	if err != nil {
		return fmt.Errorf("midtrans: cancel: %w", err)
	}
	// 404: the customer never opened the payment, so Midtrans has nothing to cancel.
	// 412: already settled/expired; the status check decides what that means.
	switch out.StatusCode {
	case "200", "404", "412":
		return nil
	}
	return fmt.Errorf("midtrans: cancel: HTTP %d code %s %s", status, out.StatusCode, out.StatusMessage)
}

func (p *Provider) Fetch(ctx context.Context, externalID, _ string) (domain.ProviderPayment, error) {
	var out transaction
	status, err := payment.DoJSON(ctx, p.http, http.MethodGet, p.apiURL+"/v2/"+url.PathEscape(externalID)+"/status", p.serverKey, nil, &out)
	if err != nil {
		return domain.ProviderPayment{}, fmt.Errorf("midtrans: status: %w", err)
	}
	if out.StatusCode == "404" {
		// Snap only creates the transaction once the customer picks a method.
		return domain.ProviderPayment{Status: domain.PaymentPending}, nil
	}
	if status/100 != 2 || out.TransactionStatus == "" {
		return domain.ProviderPayment{}, fmt.Errorf("midtrans: status: HTTP %d code %s %s", status, out.StatusCode, out.StatusMessage)
	}
	return domain.ProviderPayment{Status: mapStatus(out), Amount: parseAmount(out.GrossAmount)}, nil
}

// mapStatus turns Midtrans' transaction_status into ours. Anything unknown is
// pending, which grants nothing.
func mapStatus(t transaction) domain.PaymentStatus {
	switch strings.ToLower(t.TransactionStatus) {
	case "settlement":
		return domain.PaymentPaid
	case "capture":
		if t.FraudStatus == "" || t.FraudStatus == "accept" {
			return domain.PaymentPaid
		}
		return domain.PaymentPending
	case "deny", "failure":
		return domain.PaymentFailed
	case "cancel", "expire":
		return domain.PaymentExpired
	}
	return domain.PaymentPending
}

// parseAmount reads "19000.00"; an unreadable value is 0, which never matches.
func parseAmount(s string) int64 {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil || f < 0 {
		return 0
	}
	return int64(math.Round(f))
}

// ParseWebhook verifies signature_key = SHA512(order_id + status_code +
// gross_amount + serverKey), using the exact strings Midtrans sent.
func (p *Provider) ParseWebhook(raw []byte, _ func(string) string) (domain.WebhookEvent, error) {
	var n transaction
	if err := json.Unmarshal(raw, &n); err != nil {
		return domain.WebhookEvent{}, errors.New("midtrans: bad notification body")
	}
	sum := sha512.Sum512([]byte(n.OrderID + n.StatusCode + n.GrossAmount + p.serverKey))
	want := hex.EncodeToString(sum[:])
	if n.OrderID == "" || subtle.ConstantTimeCompare([]byte(strings.ToLower(n.SignatureKey)), []byte(want)) != 1 {
		return domain.WebhookEvent{}, domain.ErrBadSignature
	}
	return domain.WebhookEvent{EventID: n.TransactionID + "|" + n.TransactionStatus + "|" + n.StatusCode, ExternalID: n.OrderID}, nil
}

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}
