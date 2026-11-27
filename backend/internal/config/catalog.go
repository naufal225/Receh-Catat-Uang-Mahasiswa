package config

import (
	"time"

	"receh/internal/domain"
)

// ProductCatalog is what this product sells. It is plain data on purpose: edit
// the prices and labels here (a buyer of the template does exactly this) and
// the pricing page, checkout and renewals follow. Like edition.go it lives in
// its own file so branch merges never conflict on it.
//
// The plan ids stay "free" and "premium" inside the database; PlanName is what
// people read ("Pro").
//
// Amounts are whole rupiah. A yearly price of ten months is a common offer.
func ProductCatalog() domain.Catalog {
	return domain.Catalog{
		PlanName:          map[string]string{"id": "Pro", "en": "Pro"},
		Currency:          "IDR",
		GraceDays:         7,
		RenewalNoticeDays: 7,
		InvoiceTTL:        24 * time.Hour,
		Prices: []domain.PricePoint{
			{ID: "pro_monthly", Plan: domain.PlanPremium, Interval: domain.IntervalMonth, Amount: 19_000,
				Label: map[string]string{"id": "Pro bulanan", "en": "Pro monthly"}},
			{ID: "pro_yearly", Plan: domain.PlanPremium, Interval: domain.IntervalYear, Amount: 190_000,
				Label: map[string]string{"id": "Pro tahunan", "en": "Pro yearly"}},

			// Example of a further tier. It is NOT active: a tier needs limits
			// that differ from Pro (see LimitDef in the modules) before it can
			// be sold honestly.
			//
			// {ID: "family_monthly", Plan: domain.PlanPremium, Interval: domain.IntervalMonth, Amount: 39_000,
			//	Label: map[string]string{"id": "Keluarga bulanan", "en": "Family monthly"}},
		},
	}
}
