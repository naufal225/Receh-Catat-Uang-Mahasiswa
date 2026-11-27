// Package mock is a fake payment gateway for development and tests: the
// checkout "page" is a screen of this app where you choose the outcome.
//
// It keeps its state in memory, so it is only for local use; the app refuses
// to start it behind HTTPS unless MOCK_ALLOW_PROD says otherwise.
package mock

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"sync"

	"receh/internal/domain"
)

const refPrefix = "mock_"

type entry struct {
	status domain.PaymentStatus
	amount int64
}

type Provider struct {
	baseURL string
	mu      sync.Mutex
	state   map[string]*entry // by external id
}

// New returns a mock whose checkout links point at baseURL (the app's public address).
func New(baseURL string) *Provider {
	return &Provider{baseURL: strings.TrimRight(baseURL, "/"), state: map[string]*entry{}}
}

func (p *Provider) Name() string { return "mock" }

func (p *Provider) CreateCheckout(_ context.Context, req domain.CheckoutRequest) (domain.Checkout, error) {
	p.mu.Lock()
	p.state[req.ExternalID] = &entry{status: domain.PaymentPending, amount: req.Amount}
	p.mu.Unlock()
	ref := refPrefix + req.ExternalID
	return domain.Checkout{ProviderRef: ref, URL: p.baseURL + "/app/langganan/mock?ref=" + url.QueryEscape(ref)}, nil
}

func (p *Provider) Cancel(_ context.Context, externalID, _ string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if e := p.state[externalID]; e != nil && e.status == domain.PaymentPending {
		e.status = domain.PaymentExpired
	}
	return nil
}

func (p *Provider) Fetch(_ context.Context, externalID, _ string) (domain.ProviderPayment, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.state[externalID]
	if e == nil {
		// Unknown (e.g. after a restart): nothing has been paid through this mock.
		return domain.ProviderPayment{Status: domain.PaymentPending}, nil
	}
	return domain.ProviderPayment{Status: e.status, Amount: e.amount}, nil
}

// ParseWebhook accepts {"eventId": "...", "externalId": "..."}. The mock does
// not sign anything: the service never trusts the body and asks Fetch, and
// Fetch only changes through Complete.
func (p *Provider) ParseWebhook(raw []byte, _ func(string) string) (domain.WebhookEvent, error) {
	var in struct {
		EventID    string `json:"eventId"`
		ExternalID string `json:"externalId"`
	}
	if err := json.Unmarshal(raw, &in); err != nil || in.EventID == "" || in.ExternalID == "" {
		return domain.WebhookEvent{}, errors.New("mock: bad webhook body")
	}
	return domain.WebhookEvent{EventID: in.EventID, ExternalID: in.ExternalID}, nil
}

// Complete records the outcome the person picked on the mock page ("paid",
// "failed" or "expired") and returns the payment's external id.
func (p *Provider) Complete(ref, outcome string) (string, error) {
	externalID, ok := strings.CutPrefix(ref, refPrefix)
	if !ok {
		return "", errors.New("mock: unknown reference")
	}
	var st domain.PaymentStatus
	switch outcome {
	case "paid":
		st = domain.PaymentPaid
	case "failed":
		st = domain.PaymentFailed
	case "expired":
		st = domain.PaymentExpired
	default:
		return "", errors.New("mock: unknown outcome")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	e := p.state[externalID]
	if e == nil {
		return "", errors.New("mock: unknown payment (the server restarted?)")
	}
	if e.status != domain.PaymentPending {
		return "", errors.New("mock: payment already finished")
	}
	e.status = st
	return externalID, nil
}
