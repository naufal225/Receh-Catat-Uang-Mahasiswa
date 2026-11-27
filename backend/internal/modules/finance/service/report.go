package service

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
)

// ReportRenderer turns a report into a file. The PDF implementation lives in the
// platform layer so this package does not know the file format.
type ReportRenderer interface {
	Render(r *model.MonthlyReport) ([]byte, error)
}

type ReportService struct {
	txs      model.TransactionRepository
	renderer ReportRenderer
	clock    domain.Clock
}

func NewReportService(t model.TransactionRepository, r ReportRenderer, clock domain.Clock) *ReportService {
	return &ReportService{txs: t, renderer: r, clock: clock}
}

// Monthly gathers one month's numbers, querying in parallel goroutines.
func (s *ReportService) Monthly(ctx context.Context, orgID int64, owner, month string) (*model.MonthlyReport, error) {
	first, err := time.Parse("2006-01", month)
	if err != nil {
		return nil, domain.Invalid("month", "Pilih bulan laporan.")
	}
	from := first.Format(domain.DateLayout)
	to := first.AddDate(0, 1, -1).Format(domain.DateLayout)

	rep := &model.MonthlyReport{
		Month: month, Owner: owner, GeneratedAt: s.clock.Now(),
		Title: monthsLong[first.Month()-1] + " " + strconv.Itoa(first.Year()),
	}
	err = parallel(
		func() (e error) { rep.Wallets, e = s.txs.WalletSummaries(ctx, orgID, from, to); return },
		func() (e error) {
			rep.ExpenseSlices, e = s.txs.SumByCategory(ctx, orgID, 0, model.TxExpense, from, to)
			return
		},
		func() (e error) {
			rep.IncomeSlices, e = s.txs.SumByCategory(ctx, orgID, 0, model.TxIncome, from, to)
			return
		},
		func() (e error) { rep.Transactions, e = s.txs.AllBetween(ctx, orgID, from, to); return },
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
func (s *ReportService) MonthlyPDF(ctx context.Context, orgID int64, owner, month string) ([]byte, string, error) {
	rep, err := s.Monthly(ctx, orgID, owner, month)
	if err != nil {
		return nil, "", err
	}
	data, err := s.renderer.Render(rep)
	if err != nil {
		return nil, "", err
	}
	return data, fmt.Sprintf("laporan-receh-%s.pdf", month), nil
}
