package domain

import (
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// QuickDraft is what a one-line entry like "makan siang 25rb gopay kemarin"
// understood. Zero values mean "not found"; the form asks for those.
type QuickDraft struct {
	Type       TxType `json:"type"`
	Amount     int64  `json:"amount"`
	Name       string `json:"name"`
	WalletID   int64  `json:"walletId"`
	CategoryID int64  `json:"categoryId"`
	OccurredOn string `json:"occurredOn"`
}

// amountRe matches Indonesian ways to write money: 25rb, 25 ribu, 25k, 1,5jt,
// 1.5 juta, 25.000, rp25000, 25000. The unit, if any, multiplies the number.
var amountRe = regexp.MustCompile(`(?i)(?:^|\s)(?:rp\.?\s?)?(\d{1,3}(?:\.\d{3})+|\d+(?:[.,]\d+)?)\s?(rb|ribu|k|jt|juta)?(?:\s|$)`)

var (
	thousandsRe = regexp.MustCompile(`^\d{1,3}(\.\d{3})+$`)
	daysAgoRe   = regexp.MustCompile(` (\d{1,2}) hari (?:lalu|yang lalu) `)
)

// dateWords are tried in order: "kemarin lusa" before "kemarin".
var dateWords = []struct {
	word string
	days int
}{{"kemarin lusa", -2}, {"kemarin", -1}, {"hari ini", 0}, {"tadi", 0}, {"barusan", 0}}

var incomeWords = []string{"gaji", "terima", "dapat", "dapet", "masuk", "kiriman", "dikirim", "uang saku", "beasiswa", "jual", "refund", "cashback", "bonus", "honor", "transferan"}

// keywordCategory maps everyday words to the starter category they belong to,
// in the order they are tried (a slice, so the result never depends on map order).
var keywordCategory = []struct {
	category string
	words    []string
}{
	{"Makan", []string{"makan", "nasi", "ayam", "bakso", "mie", "mi ayam", "sarapan", "warteg", "soto", "sate", "padang", "geprek", "seblak", "martabak", "gorengan", "snack", "jajan"}},
	{"Minum", []string{"kopi", "teh", "es ", "boba", "jus", "susu", "minum", "air mineral", "aqua"}},
	{"Transportasi", []string{"ojek", "ojol", "gojek", "grab", "maxim", "bensin", "pertalite", "parkir", "angkot", "krl", "busway", "bus", "kereta", "tol", "taksi", "transjakarta", "mrt"}},
	{"Pulsa & Internet", []string{"pulsa", "kuota", "internet", "wifi", "paket data", "indihome"}},
	{"Listrik", []string{"listrik", "token", "pln"}},
	{"Kuliah", []string{"ukt", "spp", "fotokopi", "fotocopy", "print", "buku", "modul", "praktikum", "kuliah", "jilid"}},
	{"Langganan", []string{"netflix", "spotify", "youtube premium", "icloud", "google one", "disney", "vidio", "langganan", "canva"}},
	{"Hiburan", []string{"nonton", "bioskop", "game", "konser", "karaoke", "steam", "topup game", "top up game"}},
	{"Uang Saku", []string{"uang saku", "kiriman", "dikirim ortu", "dari ortu", "dari mama", "dari papa", "dari ibu", "dari ayah"}},
	{"Beasiswa", []string{"beasiswa", "kip"}},
	{"Kerja Sambilan", []string{"gaji", "part time", "parttime", "shift"}},
	{"Freelance", []string{"freelance", "project", "proyek", "klien", "fee desain"}},
}

// ParseAmount reads one money expression, e.g. "25rb" → 25000, "1,5jt" →
// 1500000, "25.000" → 25000. It returns 0 when s is not money.
func ParseAmount(num, unit string) int64 {
	unit = strings.ToLower(unit)
	mult := 1.0
	switch unit {
	case "rb", "ribu", "k":
		mult = 1_000
	case "jt", "juta":
		mult = 1_000_000
	}
	// "25.000" uses dots as thousands separators; "1.5" or "1,5" with a unit is a decimal.
	if unit == "" && thousandsRe.MatchString(num) {
		num = strings.ReplaceAll(num, ".", "")
	}
	num = strings.ReplaceAll(num, ",", ".")
	v, err := strconv.ParseFloat(num, 64)
	if err != nil || v <= 0 {
		return 0
	}
	return int64(v*mult + 0.5)
}

// ParseQuick reads a one-line entry. Wallets and categories are the user's own,
// so names like "GoPay" or "Jajan" are recognised as they appear in the app.
func ParseQuick(text string, wallets []Wallet, categories []Category, today time.Time) QuickDraft {
	d := QuickDraft{Type: TxExpense, OccurredOn: today.Format(DateLayout)}
	low := " " + strings.ToLower(strings.Join(strings.Fields(text), " ")) + " "
	rest := low

	// Amount: prefer an expression with a unit or of at least Rp 1.000, so in
	// "kopi 2 gelas 30rb" the 30rb is the money, not the 2.
	var pick []int
	for _, m := range amountRe.FindAllStringSubmatchIndex(low, -1) {
		if pick == nil {
			pick = m
		}
		if m[4] >= 0 || ParseAmount(low[m[2]:m[3]], "") >= 1000 {
			pick = m
			break
		}
	}
	if pick != nil {
		unit := ""
		if pick[4] >= 0 {
			unit = low[pick[4]:pick[5]]
		}
		d.Amount = ParseAmount(low[pick[2]:pick[3]], unit)
		rest = strings.Replace(rest, " "+strings.TrimSpace(low[pick[0]:pick[1]])+" ", " ", 1)
	}

	// Date words.
	for _, dw := range dateWords {
		if strings.Contains(rest, " "+dw.word+" ") {
			d.OccurredOn = today.AddDate(0, 0, dw.days).Format(DateLayout)
			rest = strings.Replace(rest, " "+dw.word+" ", " ", 1)
			break
		}
	}
	if m := daysAgoRe.FindStringSubmatch(rest); m != nil {
		n, _ := strconv.Atoi(m[1])
		d.OccurredOn = today.AddDate(0, 0, -n).Format(DateLayout)
		rest = strings.Replace(rest, m[0], " ", 1)
	}

	// Income or expense.
	for _, w := range incomeWords {
		if strings.Contains(low, " "+w) {
			d.Type = TxIncome
			break
		}
	}
	if strings.HasPrefix(strings.TrimSpace(text), "+") {
		d.Type = TxIncome
		rest = strings.Replace(rest, "+", " ", 1)
	}

	// Wallet: the longest wallet name found, so "bca tabungan" beats "bca".
	best := ""
	for _, w := range wallets {
		name := strings.ToLower(w.Name)
		if strings.Contains(rest, " "+name+" ") && len(name) > len(best) {
			best, d.WalletID = name, w.ID
		}
	}
	if best != "" {
		rest = strings.Replace(rest, " "+best+" ", " ", 1)
		rest = strings.Replace(rest, " pakai ", " ", 1)
		rest = strings.Replace(rest, " pake ", " ", 1)
		rest = strings.Replace(rest, " dari ", " ", 1)
		rest = strings.Replace(rest, " via ", " ", 1)
	}

	// Category: a category named in the text, else a keyword of a starter one.
	byName := map[string]Category{}
	for _, c := range categories {
		if c.Type == d.Type {
			byName[strings.ToLower(c.Name)] = c
		}
	}
	for name, c := range byName {
		if strings.Contains(low, " "+name+" ") {
			d.CategoryID = c.ID
			break
		}
	}
	if d.CategoryID == 0 {
	outer:
		for _, kc := range keywordCategory {
			c, ok := byName[strings.ToLower(kc.category)]
			if !ok {
				continue
			}
			for _, w := range kc.words {
				if strings.Contains(low, " "+w) {
					d.CategoryID = c.ID
					break outer
				}
			}
		}
	}

	d.Name = capitalize(strings.Join(strings.Fields(rest), " "))
	return d
}

func capitalize(s string) string {
	for i, r := range s {
		return string(unicode.ToUpper(r)) + s[i+len(string(r)):]
	}
	return s
}

func lowerTrim(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }
