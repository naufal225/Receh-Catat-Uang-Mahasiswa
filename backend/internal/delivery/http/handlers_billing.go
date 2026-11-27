package http

import (
	"errors"
	"io"
	"log"
	"net/http"
	"time"

	"receh/internal/domain"
	"receh/internal/platform/ratelimit"
	"receh/internal/usecase"
)

// Gateways retry on their own; this only stops a flood from one address.
var webhookLimit = ratelimit.New(300, time.Minute)

func (s *Server) billingCatalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.svc.Billing.PublicCatalog())
}

func (s *Server) billingOverview(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	o, err := s.svc.Billing.Overview(r.Context(), u, OrgFrom(r.Context()).ID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, o)
}

func (s *Server) billingCheckout(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	var in struct {
		PriceID string `json:"priceId"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	p, err := s.svc.Billing.Checkout(r.Context(), u, OrgFrom(r.Context()).ID, in.PriceID, s.clientInfo(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) billingCancelPayment(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if err := s.svc.Billing.CancelPayment(r.Context(), u, OrgFrom(r.Context()).ID, id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) billingSyncPayment(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := s.svc.Billing.SyncPayment(r.Context(), u, OrgFrom(r.Context()).ID, id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) billingSetRenewal(stop bool) authed {
	return func(w http.ResponseWriter, r *http.Request, u *domain.User, _ *domain.Session) {
		if err := s.svc.Billing.SetRenewal(r.Context(), u, OrgFrom(r.Context()).ID, stop, s.clientInfo(r)); err != nil {
			writeError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// billingWebhook receives gateway notifications. It is public: authenticity is
// the provider's signature, checked inside ParseWebhook.
func (s *Server) billingWebhook(w http.ResponseWriter, r *http.Request) {
	ip := s.clientInfo(r).IP
	if webhookLimit.Blocked(ip) {
		writeJSON(w, http.StatusTooManyRequests, errorBody{Code: "TOO_MANY", Message: "Terlalu banyak permintaan."})
		return
	}
	webhookLimit.Hit(ip)
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		writeJSON(w, http.StatusRequestEntityTooLarge, errorBody{Code: "TOO_LARGE", Message: "Isi terlalu besar."})
		return
	}
	err = s.svc.Billing.HandleWebhook(r.Context(), r.PathValue("provider"), raw, r.Header.Get)
	var app *domain.AppError
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, domain.ErrBadSignature):
		writeJSON(w, http.StatusUnauthorized, errorBody{Code: "BAD_SIGNATURE", Message: "Tanda tangan tidak valid."})
	case errors.As(err, &app):
		writeError(w, err)
	default:
		// A failure on our side: 5xx makes the gateway deliver again.
		log.Printf("billing webhook %s: %v", r.PathValue("provider"), err)
		writeJSON(w, http.StatusInternalServerError, errorBody{Code: "INTERNAL", Message: "Gagal memproses."})
	}
}

// billingMockComplete is the fake checkout page's button (mock provider only).
func (s *Server) billingMockComplete(w http.ResponseWriter, r *http.Request) {
	if err := s.svc.Billing.MockComplete(r.Context(), r.PathValue("ref"), r.PathValue("outcome")); err != nil {
		var app *domain.AppError
		if errors.As(err, &app) {
			writeError(w, err)
		} else {
			log.Printf("billing mock: %v", err)
			writeJSON(w, http.StatusInternalServerError, errorBody{Code: "INTERNAL", Message: "Gagal memproses."})
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Admin ------------------------------------------------------------------------

func (s *Server) adminPayments(w http.ResponseWriter, r *http.Request, _ *domain.User, _ *domain.Session) {
	status := domain.PaymentStatus(r.URL.Query().Get("status"))
	switch status {
	case "", domain.PaymentPending, domain.PaymentPaid, domain.PaymentExpired, domain.PaymentFailed:
	default:
		writeError(w, domain.Invalid("status", "Status tidak dikenal."))
		return
	}
	page, err := s.svc.Billing.AdminPayments(r.Context(), status, queryInt(r, "page"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (s *Server) adminSyncPayment(w http.ResponseWriter, r *http.Request, _ *domain.User, _ *domain.Session) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	p, err := s.svc.Billing.AdminSyncPayment(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) adminBillingStatus(w http.ResponseWriter, _ *http.Request, _ *domain.User, _ *domain.Session) {
	writeJSON(w, http.StatusOK, struct {
		Enabled bool `json:"enabled"`
		usecase.IntegrationStatus
	}{s.svc.Billing.Enabled(), s.svc.Billing.IntegrationStatus()})
}
