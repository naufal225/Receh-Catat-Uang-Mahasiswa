// Package xendit talks to the Xendit Invoice API (v2). It uses the REST API
// directly, no SDK.
//
// NOTE: the field names below come from the long-standing v2 Invoice API. The
// current docs pages could not be read while this was written, so verify
// them against the sandbox (see docs/SETUP-CHECKLIST.md) before going live.
package xendit

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/payment"
)

type Provider struct {
	secretKey     string
	callbackToken string
	baseURL       string
	http          *http.Client
}

func New(secretKey, callbackToken string) *Provider {
	return &Provider{secretKey: secretKey, callbackToken: callbackToken, baseURL: "https://api.xendit.co", http: payment.Client}
}

// WithBaseURL points the adapter at another host (tests use a local server).
func (p *Provider) WithBaseURL(u string) *Provider {
	p.baseURL = strings.TrimRight(u, "/")
	return p
}

func (p *Provider) Name() string { return "xendit" }

type invoice struct {
	ID         string  `json:"id"`
	ExternalID string  `json:"external_id"`
	Status     string  `json:"status"`
	InvoiceURL string  `json:"invoice_url"`
	Amount     float64 `json:"amount"`
	PaidAmount float64 `json:"paid_amount"`
	ErrorCode  string  `json:"error_code"`
	Message    string  `json:"message"`
}

func (p *Provider) CreateCheckout(ctx context.Context, req domain.CheckoutRequest) (domain.Checkout, error) {
	seconds := max(int(time.Until(req.ExpiresAt).Seconds()), 60)
	body := map[string]any{
		"external_id":          req.ExternalID,
		"amount":               req.Amount,
		"currency":             req.Currency,
		"payer_email":          req.PayerEmail,
		"description":          req.Description,
		"invoice_duration":     seconds,
		"success_redirect_url": req.ReturnURL,
		"failure_redirect_url": req.ReturnURL,
	}
	var out invoice
	status, err := payment.DoJSON(ctx, p.http, http.MethodPost, p.baseURL+"/v2/invoices", p.secretKey, body, &out)
	if err != nil {
		return domain.Checkout{}, fmt.Errorf("xendit: create invoice: %w", err)
	}
	if status/100 != 2 || out.InvoiceURL == "" {
		return domain.Checkout{}, fmt.Errorf("xendit: create invoice: HTTP %d %s %s", status, out.ErrorCode, out.Message)
	}
	return domain.Checkout{ProviderRef: out.ID, URL: out.InvoiceURL}, nil
}

func (p *Provider) Cancel(ctx context.Context, _, providerRef string) error {
	if providerRef == "" {
		return nil
	}
	var out invoice
	status, err := payment.DoJSON(ctx, p.http, http.MethodPost, p.baseURL+"/invoices/"+url.PathEscape(providerRef)+"/expire!", p.secretKey, nil, &out)
	if err != nil {
		return fmt.Errorf("xendit: expire invoice: %w", err)
	}
	// Already expired or paid is fine: the status check decides what that means.
	if status/100 != 2 && status != http.StatusBadRequest && status != http.StatusNotFound {
		return fmt.Errorf("xendit: expire invoice: HTTP %d %s", status, out.Message)
	}
	return nil
}

func (p *Provider) Fetch(ctx context.Context, externalID, providerRef string) (domain.ProviderPayment, error) {
	var inv invoice
	if providerRef != "" {
		status, err := payment.DoJSON(ctx, p.http, http.MethodGet, p.baseURL+"/v2/invoices/"+url.PathEscape(providerRef), p.secretKey, nil, &inv)
		if err != nil {
			return domain.ProviderPayment{}, fmt.Errorf("xendit: get invoice: %w", err)
		}
		if status/100 != 2 {
			return domain.ProviderPayment{}, fmt.Errorf("xendit: get invoice: HTTP %d %s", status, inv.Message)
		}
	} else {
		var list []invoice
		status, err := payment.DoJSON(ctx, p.http, http.MethodGet, p.baseURL+"/v2/invoices?external_id="+url.QueryEscape(externalID), p.secretKey, nil, &list)
		if err != nil {
			return domain.ProviderPayment{}, fmt.Errorf("xendit: list invoices: %w", err)
		}
		if status/100 != 2 || len(list) == 0 {
			return domain.ProviderPayment{}, fmt.Errorf("xendit: invoice %s not found (HTTP %d)", externalID, status)
		}
		inv = list[0]
	}
	return domain.ProviderPayment{Status: mapStatus(inv.Status), Amount: int64(math.Round(inv.Amount))}, nil
}

// mapStatus: PAID and SETTLED are money received; anything unknown grants nothing.
func mapStatus(s string) domain.PaymentStatus {
	switch strings.ToUpper(s) {
	case "PAID", "SETTLED":
		return domain.PaymentPaid
	case "EXPIRED":
		return domain.PaymentExpired
	}
	return domain.PaymentPending
}

// ParseWebhook checks the x-callback-token header against the token from the
// Xendit dashboard (constant time) and reads which invoice changed.
func (p *Provider) ParseWebhook(raw []byte, header func(string) string) (domain.WebhookEvent, error) {
	if p.callbackToken == "" || subtle.ConstantTimeCompare([]byte(header("x-callback-token")), []byte(p.callbackToken)) != 1 {
		return domain.WebhookEvent{}, domain.ErrBadSignature
	}
	var n invoice
	if err := json.Unmarshal(raw, &n); err != nil || n.ExternalID == "" {
		return domain.WebhookEvent{}, errors.New("xendit: bad callback body")
	}
	return domain.WebhookEvent{EventID: n.ID + "|" + strings.ToUpper(n.Status), ExternalID: n.ExternalID}, nil
}
