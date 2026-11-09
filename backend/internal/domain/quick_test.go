package domain

import (
	"testing"
	"time"
)

func TestParseAmount(t *testing.T) {
	for _, c := range []struct {
		num, unit string
		want      int64
	}{
		{"25", "rb", 25_000}, {"25", "ribu", 25_000}, {"25", "k", 25_000}, {"25", "K", 25_000},
		{"1,5", "jt", 1_500_000}, {"1.5", "juta", 1_500_000}, {"2", "jt", 2_000_000},
		{"25.000", "", 25_000}, {"1.250.000", "", 1_250_000}, {"25000", "", 25_000},
		{"12,5", "rb", 12_500}, {"0", "rb", 0}, {"abc", "", 0},
	} {
		if got := ParseAmount(c.num, c.unit); got != c.want {
			t.Errorf("ParseAmount(%q, %q) = %d, want %d", c.num, c.unit, got, c.want)
		}
	}
}

func TestParseQuick(t *testing.T) {
	today := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	wallets := []Wallet{{ID: 1, Name: "BCA"}, {ID: 2, Name: "GoPay"}, {ID: 3, Name: "BCA Tabungan"}, {ID: 4, Name: "Tunai"}}
	cats := []Category{
		{ID: 10, Name: "Makan", Type: TxExpense}, {ID: 11, Name: "Minum", Type: TxExpense},
		{ID: 12, Name: "Transportasi", Type: TxExpense}, {ID: 13, Name: "Langganan", Type: TxExpense},
		{ID: 14, Name: "Jajan", Type: TxExpense}, {ID: 15, Name: "Pulsa & Internet", Type: TxExpense},
		{ID: 20, Name: "Uang Saku", Type: TxIncome}, {ID: 21, Name: "Kerja Sambilan", Type: TxIncome},
	}
	for _, c := range []struct {
		in   string
		want QuickDraft
	}{
		{"makan siang 25rb gopay", QuickDraft{TxExpense, 25_000, "Makan siang", 2, 10, "2026-01-15"}},
		{"Kopi susu 18k pakai BCA kemarin", QuickDraft{TxExpense, 18_000, "Kopi susu", 1, 11, "2026-01-14"}},
		{"ojek ke kampus 12.000 tunai", QuickDraft{TxExpense, 12_000, "Ojek ke kampus", 4, 12, "2026-01-15"}},
		{"netflix 54rb bca tabungan", QuickDraft{TxExpense, 54_000, "Netflix", 3, 13, "2026-01-15"}},
		{"kiriman uang saku 1,5jt bca", QuickDraft{TxIncome, 1_500_000, "Kiriman uang saku", 1, 20, "2026-01-15"}},
		{"gaji part time 750rb 3 hari lalu", QuickDraft{TxIncome, 750_000, "Gaji part time", 0, 21, "2026-01-12"}},
		{"kopi 2 gelas 30rb", QuickDraft{TxExpense, 30_000, "Kopi 2 gelas", 0, 11, "2026-01-15"}},
		{"jajan seblak 15rb kemarin lusa", QuickDraft{TxExpense, 15_000, "Jajan seblak", 0, 14, "2026-01-13"}},
		{"beli kuota 50 ribu", QuickDraft{TxExpense, 50_000, "Beli kuota", 0, 15, "2026-01-15"}},
		{"sesuatu", QuickDraft{TxExpense, 0, "Sesuatu", 0, 0, "2026-01-15"}},
	} {
		if got := ParseQuick(c.in, wallets, cats, today); got != c.want {
			t.Errorf("ParseQuick(%q)\n got  %+v\n want %+v", c.in, got, c.want)
		}
	}
}
