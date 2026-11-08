package usecase

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"receh/internal/domain"
)

// ReportRenderer turns a report into a file. The PDF implementation lives in the
// platform layer so this package does not know the file format.
type ReportRenderer interface {
	Render(r *domain.MonthlyReport) ([]byte, error)
}

type ReportService struct {
	txs      domain.TransactionRepository
	renderer ReportRenderer
	clock    domain.Clock
}

func NewReportService(t domain.TransactionRepository, r ReportRenderer, clock domain.Clock) *ReportService {
	return &ReportService{txs: t, renderer: r, clock: clock}
}

// Monthly gathers one month's numbers, querying in parallel goroutines.
func (s *ReportService) Monthly(ctx context.Context, u *domain.User, month string) (*domain.MonthlyReport, error) {
	first, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, domain.Invalid("month", "Pilih bulan laporan.")
	}
	from := first.Format(domain.DateLayout)
	to := first.AddDate(0, 1, -1).Format(domain.DateLayout)

	rep := &domain.MonthlyReport{
		Month: month, Owner: u.Name, GeneratedAt: s.clock.Now(),
		Title: monthsLong[first.Month()-1] + " " + strconv.Itoa(first.Year()),
	}
	err = parallel(
		func() (e error) { rep.Wallets, e = s.txs.WalletSummaries(ctx, u.ID, from, to); return },
		func() (e error) {
			rep.ExpenseSlices, e = s.txs.SumByCategory(ctx, u.ID, 0, domain.TxExpense, from, to)
			return
		},
		func() (e error) {
			rep.IncomeSlices, e = s.txs.SumByCategory(ctx, u.ID, 0, domain.TxIncome, from, to)
			return
		},
		func() (e error) { rep.Transactions, e = s.txs.AllBetween(ctx, u.ID, from, to); return },
	)
	if err != nil {
		return nil, err
	}
	for _, c := range rep.ExpenseSlices {
		rep.Totals.Expense += c.Total
	}
	for _, c := range rep.IncomeSlices {
		rep.Totals.Income += c.Total
	}
	return rep, nil
}

// MonthlyPDF returns the rendered file and a download name.
func (s *ReportService) MonthlyPDF(ctx context.Context, u *domain.User, month string) ([]byte, string, error) {
	rep, err := s.Monthly(ctx, u, month)
	if err != nil {
		return nil, "", err
	}
	data, err := s.renderer.Render(rep)
	if err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("laporan-receh-%s.pdf", month), nil
}
