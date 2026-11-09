package domain

import (
	"testing"
	"time"
)

func TestParseMoneyCell(t *testing.T) {
	for _, c := range []struct {
		in   string
		v    int64
		sign int
	}{
		{"1.250.000,00", 1_250_000, 0}, {"1,250,000.00", 1_250_000, 0}, {"25000", 25_000, 0},
		{"-54.000", 54_000, -1}, {"(15.000)", 15_000, -1}, {"Rp 18.500", 18_500, 0},
		{"500,000.00 CR", 500_000, 1}, {"12,500.00 DB", 12_500, -1}, {"+750000", 750_000, 1},
		{"99,99", 100, 0}, {"", 0, 0}, {"abc", 0, 0}, {"-", 0, 0},
	} {
		v, sign := parseMoneyCell(c.in)
		if v != c.v || sign != c.sign {
			t.Errorf("parseMoneyCell(%q) = %d,%d want %d,%d", c.in, v, sign, c.v, c.sign)
		}
	}
}

func TestParseStatementDate(t *testing.T) {
	today := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)
	for in, want := range map[string]string{
		"14/01/2026": "2026-01-14", "4/1/26": "2026-01-04", "14-01-2026": "2026-01-14",
		"2026-01-14": "2026-01-14", "2026-01-14 08:30:00": "2026-01-14", "14 Jan 2026": "2026-01-14",
		"14 Januari 2026": "2026-01-14", "02 Des 2025": "2025-12-02", "14/01": "2026-01-14",
		"20/12": "2025-12-20", "31/02/2026": "", "PEND": "", "Saldo Awal": "",
	} {
		if got := parseStatementDate(in, today); got != want {
			t.Errorf("parseStatementDate(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseStatementFormats(t *testing.T) {
	today := time.Date(2026, 1, 15, 0, 0, 0, 0, time.UTC)

	// BCA style: title lines, DD/MM dates, one amount column marked CR/DB.
	bca := "Informasi Rekening - Mutasi Rekening\nNo. rekening : 1234567890\n\n" +
		"Tanggal Transaksi,Keterangan,Cabang,Jumlah,Saldo\n" +
		"'12/01,TRSF E-BANKING DB GOPAY TOPUP,0000,\"100,000.00 DB\",\"1,400,000.00\"\n" +
		"'13/01,KR OTOMATIS BEASISWA,0000,\"2,000,000.00 CR\",\"3,400,000.00\"\n" +
		"'14/01,PEND,0000,,\n" +
		"Saldo Awal,,,,\"1,500,000.00\"\n"
	p, err := ParseStatement([]byte(bca), nil, today)
	if err != nil {
		t.Fatal(err)
	}
	if p.Mapping.Date != 0 || p.Mapping.Description != 1 || p.Mapping.Amount != 3 || len(p.Rows) != 2 || p.Skipped != 2 {
		t.Fatalf("BCA: mapping %+v rows %+v skipped %d", p.Mapping, p.Rows, p.Skipped)
	}
	if r := p.Rows[0]; r.Date != "2026-01-12" || r.Amount != 100_000 || r.Type != TxExpense || r.Name != "TRSF E-BANKING DB GOPAY TOPUP" || r.Line != 5 {
		t.Fatalf("BCA row 0 %+v", r)
	}
	if r := p.Rows[1]; r.Amount != 2_000_000 || r.Type != TxIncome {
		t.Fatalf("BCA row 1 %+v", r)
	}

	// E-wallet style: semicolons, split Debit/Kredit columns, ID number format.
	ew := "Waktu;Deskripsi;Debit;Kredit\n2026-01-10 12:01:00;Bayar GoFood;45.500;\n2026-01-11 09:00:00;Top Up;;100.000\n"
	p, err = ParseStatement([]byte(ew), nil, today)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Rows) != 2 || p.Rows[0].Amount != 45_500 || p.Rows[0].Type != TxExpense || p.Rows[1].Type != TxIncome || p.Rows[1].Date != "2026-01-11" {
		t.Fatalf("e-wallet %+v %+v", p.Mapping, p.Rows)
	}

	// No header at all: columns guessed from content, signed amounts.
	raw := "14/01/2026,Makan siang,-25000\n15/01/2026,Uang saku,+500000\n"
	p, err = ParseStatement([]byte(raw), nil, today)
	if err != nil || len(p.Rows) != 2 || p.Rows[0].Type != TxExpense || p.Rows[1].Type != TxIncome {
		t.Fatalf("headerless %+v %+v %v", p.Mapping, p.Rows, err)
	}

	// Unknown columns: no rows, headers returned so the person can map them.
	odd := "A,B,C\nx,y,z\n"
	p, _ = ParseStatement([]byte(odd), nil, today)
	if len(p.Rows) != 0 {
		t.Fatalf("odd file produced rows %+v", p.Rows)
	}
	m := StatementMapping{Date: 0, Description: 1, Amount: 2, Debit: -1, Credit: -1, HeaderRow: 0}
	p, _ = ParseStatement([]byte("Tgl,Apa,Berapa\n01/01/2026,Kopi,-15000\n"), &m, today)
	if len(p.Rows) != 1 || p.Rows[0].Amount != 15_000 {
		t.Fatalf("explicit mapping %+v", p.Rows)
	}
}
