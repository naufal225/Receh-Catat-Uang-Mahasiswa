package domain

import (
	"bytes"
	"encoding/csv"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// StatementMapping says which CSV column holds what (0-based, -1 = absent).
// Amount is one signed/marked column; Debit and Credit are the split form.
type StatementMapping struct {
	Date        int `json:"date"`
	Description int `json:"description"`
	Amount      int `json:"amount"`
	Debit       int `json:"debit"`
	Credit      int `json:"credit"`
	// HeaderRow is the 0-based row holding column names, -1 if none.
	HeaderRow int `json:"headerRow"`
}

// StatementRow is one parsed mutation from a bank or e-wallet export.
type StatementRow struct {
	Line   int    `json:"line"` // 1-based line in the file
	Date   string `json:"date"` // YYYY-MM-DD
	Name   string `json:"name"`
	Amount int64  `json:"amount"`
	Type   TxType `json:"type"`
}

type ParsedStatement struct {
	Headers []string         `json:"headers"`
	Mapping StatementMapping `json:"mapping"`
	Rows    []StatementRow   `json:"rows"`
	// Skipped counts lines that were not mutations (balances, titles, blanks).
	Skipped int `json:"skipped"`
}

var ErrStatementUnreadable = errors.New("statement unreadable")

var headerWords = map[string][]string{
	"date":        {"tanggal", "tgl", "date", "waktu", "posting", "transaction date"},
	"description": {"keterangan", "deskripsi", "description", "uraian", "remark", "detail", "berita", "catatan", "merchant", "nama"},
	"amount":      {"jumlah", "nominal", "amount", "mutasi", "nilai"},
	"debit":       {"debit", "debet", "keluar", "pengeluaran", "uang keluar"},
	"credit":      {"kredit", "credit", "masuk", "pemasukan", "uang masuk"},
}

// ParseStatement reads a CSV export. With a nil mapping it detects the
// delimiter, the header row and the columns; a mapping overrides detection.
// today resolves dates written without a year ("15/01", as BCA does).
func ParseStatement(data []byte, mapping *StatementMapping, today time.Time) (*ParsedStatement, error) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // UTF-8 BOM from Excel
	r := csv.NewReader(bytes.NewReader(data))
	r.Comma = sniffDelimiter(data)
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	r.TrimLeadingSpace = true
	var records [][]string
	var lines []int // file line of each record (blank lines are skipped by the reader)
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, ErrStatementUnreadable
		}
		line, _ := r.FieldPos(0)
		records = append(records, rec)
		lines = append(lines, line)
	}
	if len(records) == 0 {
		return nil, ErrStatementUnreadable
	}

	out := &ParsedStatement{Rows: []StatementRow{}}
	if mapping != nil {
		out.Mapping = *mapping
	} else {
		out.Mapping = detectMapping(records)
	}
	m := out.Mapping
	if m.HeaderRow >= 0 && m.HeaderRow < len(records) {
		out.Headers = records[m.HeaderRow]
	}
	if m.Date < 0 || m.Description < 0 || (m.Amount < 0 && m.Debit < 0 && m.Credit < 0) {
		return out, nil // The caller shows the columns so the person can map them.
	}
	for i, rec := range records {
		if i <= m.HeaderRow {
			continue
		}
		row, ok := parseRow(rec, m, today)
		if !ok {
			out.Skipped++
			continue
		}
		row.Line = lines[i]
		out.Rows = append(out.Rows, row)
	}
	return out, nil
}

func sniffDelimiter(data []byte) rune {
	first := data
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		first = data[:min(len(data), i+1+bytes.IndexByte(data[i+1:], '\n')+1)] // two lines
	}
	best, n := ',', bytes.Count(first, []byte(","))
	for _, d := range []rune{';', '\t', '|'} {
		if c := bytes.Count(first, []byte(string(d))); c > n {
			best, n = d, c
		}
	}
	return best
}

func detectMapping(records [][]string) StatementMapping {
	m := StatementMapping{Date: -1, Description: -1, Amount: -1, Debit: -1, Credit: -1, HeaderRow: -1}
	for i, rec := range records[:min(len(records), 15)] {
		cand := StatementMapping{Date: -1, Description: -1, Amount: -1, Debit: -1, Credit: -1, HeaderRow: i}
		for j, cell := range rec {
			c := strings.ToLower(strings.TrimSpace(cell))
			for field, words := range headerWords {
				for _, w := range words {
					if c == w || strings.HasPrefix(c, w+" ") || strings.HasSuffix(c, " "+w) {
						switch field {
						case "date":
							setOnce(&cand.Date, j)
						case "description":
							setOnce(&cand.Description, j)
						case "amount":
							setOnce(&cand.Amount, j)
						case "debit":
							setOnce(&cand.Debit, j)
						case "credit":
							setOnce(&cand.Credit, j)
						}
					}
				}
			}
		}
		if cand.Date >= 0 && cand.Description >= 0 && (cand.Amount >= 0 || cand.Debit >= 0 || cand.Credit >= 0) {
			return cand
		}
	}
	// No header: guess from content (first row whose cells look like a date and money).
	for _, rec := range records[:min(len(records), 15)] {
		cand := StatementMapping{Date: -1, Description: -1, Amount: -1, Debit: -1, Credit: -1, HeaderRow: -1}
		longest := 0
		for j, cell := range rec {
			switch {
			case cand.Date < 0 && parseStatementDate(cell, time.Now()) != "":
				cand.Date = j
			case cand.Amount < 0 && moneyCellRe.MatchString(strings.TrimSpace(cell)):
				cand.Amount = j
			case len(strings.TrimSpace(cell)) > longest:
				longest, cand.Description = len(strings.TrimSpace(cell)), j
			}
		}
		if cand.Date >= 0 && cand.Amount >= 0 && cand.Description >= 0 {
			return cand
		}
	}
	return m
}

func setOnce(p *int, v int) {
	if *p < 0 {
		*p = v
	}
}

var (
	moneyCellRe = regexp.MustCompile(`^[-+(]?\s?(?:rp\.?\s?)?\d[\d.,]*\)?\s?(?:cr|db|d|k)?$`)
	monthNames  = map[string]time.Month{
		"jan": 1, "januari": 1, "january": 1, "feb": 2, "februari": 2, "february": 2, "mar": 3, "maret": 3, "march": 3,
		"apr": 4, "april": 4, "mei": 5, "may": 5, "jun": 6, "juni": 6, "june": 6, "jul": 7, "juli": 7, "july": 7,
		"agu": 8, "ags": 8, "agustus": 8, "aug": 8, "august": 8, "sep": 9, "september": 9, "okt": 10, "oktober": 10,
		"oct": 10, "october": 10, "nov": 11, "november": 11, "des": 12, "desember": 12, "dec": 12, "december": 12,
	}
	dmyRe   = regexp.MustCompile(`^(\d{1,2})[/\-.](\d{1,2})(?:[/\-.](\d{2,4}))?`)
	ymdRe   = regexp.MustCompile(`^(\d{4})-(\d{1,2})-(\d{1,2})`)
	dMonYRe = regexp.MustCompile(`^(\d{1,2})[\s\-]([A-Za-z]+)[\s\-](\d{2,4})`)
)

// parseStatementDate understands DD/MM/YYYY, DD-MM-YY, DD/MM (current year,
// or last year if that would be in the future), YYYY-MM-DD and "02 Jan 2026".
func parseStatementDate(s string, today time.Time) string {
	s = strings.TrimSpace(strings.Trim(s, `'"`))
	build := func(y, mo, d int) string {
		if mo < 1 || mo > 12 || d < 1 || d > 31 {
			return ""
		}
		t := time.Date(y, time.Month(mo), d, 0, 0, 0, 0, time.UTC)
		if t.Day() != d {
			return ""
		}
		return t.Format(DateLayout)
	}
	if m := ymdRe.FindStringSubmatch(s); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		return build(y, mo, d)
	}
	if m := dMonYRe.FindStringSubmatch(s); m != nil {
		mo, ok := monthNames[strings.ToLower(m[2])]
		if !ok {
			return ""
		}
		d, _ := strconv.Atoi(m[1])
		y, _ := strconv.Atoi(m[3])
		if y < 100 {
			y += 2000
		}
		return build(y, int(mo), d)
	}
	if m := dmyRe.FindStringSubmatch(s); m != nil {
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if m[3] == "" {
			y := today.Year()
			if time.Month(mo) > today.Month() {
				y-- // "20/12" read in January is last December
			}
			return build(y, mo, d)
		}
		y, _ := strconv.Atoi(m[3])
		if y < 100 {
			y += 2000
		}
		return build(y, mo, d)
	}
	return ""
}

// parseMoneyCell reads amounts in either convention ("1.250.000,00" or
// "1,250,000.00"), with an optional sign, parentheses, Rp, or a DB/CR marker.
// It returns the amount in whole rupiah and a sign: -1 out, +1 in, 0 unknown.
func parseMoneyCell(s string) (int64, int) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || s == "-" {
		return 0, 0
	}
	sign := 0
	switch {
	case strings.HasSuffix(s, "cr"), strings.HasSuffix(s, " k"):
		sign = 1
	case strings.HasSuffix(s, "db"), strings.HasSuffix(s, " d"):
		sign = -1
	}
	s = strings.TrimRight(s, "crdbk ")
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, ")") {
		sign, s = -1, s[1:len(s)-1]
	}
	if strings.HasPrefix(s, "-") {
		sign, s = -1, s[1:]
	} else if strings.HasPrefix(s, "+") {
		sign, s = 1, s[1:]
	}
	s = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(s, "rp."), "rp"))
	if s == "" || strings.Trim(s, "0123456789.,") != "" {
		return 0, 0
	}
	dot, comma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")
	decimal := -1
	switch {
	case dot >= 0 && comma >= 0:
		decimal = max(dot, comma) // the later separator is the decimal one
	case comma >= 0 && len(s)-comma-1 <= 2:
		decimal = comma // "50,00"
	case dot >= 0 && len(s)-dot-1 <= 2 && strings.Count(s, ".") == 1:
		decimal = dot // "50.5"
	}
	whole := s
	if decimal >= 0 {
		whole = s[:decimal]
	}
	whole = strings.NewReplacer(".", "", ",", "").Replace(whole)
	v, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, 0
	}
	if decimal >= 0 && decimal+1 < len(s) && s[decimal+1] >= '5' {
		v++ // round half up to whole rupiah
	}
	return v, sign
}

func cell(rec []string, i int) string {
	if i < 0 || i >= len(rec) {
		return ""
	}
	return rec[i]
}

func parseRow(rec []string, m StatementMapping, today time.Time) (StatementRow, bool) {
	row := StatementRow{Type: TxExpense}
	if row.Date = parseStatementDate(cell(rec, m.Date), today); row.Date == "" {
		return row, false
	}
	row.Name = strings.Join(strings.Fields(cell(rec, m.Description)), " ")
	if row.Name == "" {
		row.Name = "Transaksi impor"
	}
	if len([]rune(row.Name)) > 80 {
		row.Name = string([]rune(row.Name)[:80])
	}
	if m.Amount >= 0 {
		v, sign := parseMoneyCell(cell(rec, m.Amount))
		if v <= 0 {
			return row, false
		}
		row.Amount = v
		if sign > 0 {
			row.Type = TxIncome
		}
		return row, true
	}
	debit, _ := parseMoneyCell(cell(rec, m.Debit))
	credit, _ := parseMoneyCell(cell(rec, m.Credit))
	switch {
	case debit > 0:
		row.Amount = debit
	case credit > 0:
		row.Amount, row.Type = credit, TxIncome
	default:
		return row, false
	}
	return row, true
}
