package report

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"receh/internal/domain"
	"receh/internal/modules/finance/model"
	"receh/internal/platform/pdf"
)

var (
	ink     = pdf.Hex("#16302b")
	muted   = pdf.Hex("#5c6b66")
	hair    = pdf.Hex("#e4dfd3")
	paper   = pdf.Hex("#fbf7ee")
	white   = pdf.Hex("#ffffff")
	income  = pdf.Hex("#0f7d57")
	expense = pdf.Hex("#c44a1c")
	accent  = pdf.Hex("#f2b705")
)

// slotColors is the print step of the same categorical slots the web UI uses.
var slotColors = map[string]pdf.RGB{
	"c0": pdf.Hex("#898781"), "c1": pdf.Hex("#2a78d6"), "c2": pdf.Hex("#eb6834"), "c3": pdf.Hex("#1baf7a"), "c4": pdf.Hex("#eda100"),
	"c5": pdf.Hex("#e87ba4"), "c6": pdf.Hex("#008300"), "c7": pdf.Hex("#6250d6"), "c8": pdf.Hex("#e34948"),
}

const (
	marginX = 40.0
	bottomY = pdf.PageH - 56
)

// ReportRenderer implements usecase.ReportRenderer.
type ReportRenderer struct{ AppName string }

func Rupiah(v int64) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	s := strconv.FormatInt(v, 10)
	var parts []string
	for len(s) > 3 {
		parts = append([]string{s[len(s)-3:]}, parts...)
		s = s[:len(s)-3]
	}
	parts = append([]string{s}, parts...)
	return sign + "Rp " + strings.Join(parts, ".")
}

var monthsShort = []string{"Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Agu", "Sep", "Okt", "Nov", "Des"}

func shortDate(iso string) string {
	d, err := domain.ParseDate(iso)
	if err != nil {
		return iso
	}
	return fmt.Sprintf("%d %s", d.Day(), monthsShort[d.Month()-1])
}

func (rr ReportRenderer) Render(r *model.MonthlyReport) ([]byte, error) {
	d := pdf.New("Laporan " + rr.AppName + " " + r.Title)
	d.AddPage()
	y := header(d, r, rr.AppName)
	y = statTiles(d, r, y)
	y = walletTable(d, r, y)
	y = breakdown(d, "Pengeluaran per kategori", r.ExpenseSlices, r.Totals.Expense, y)
	y = breakdown(d, "Pemasukan per kategori", r.IncomeSlices, r.Totals.Income, y)
	transactionTable(d, r, y)
	footers(d, r, rr.AppName)
	return d.Bytes()
}

func header(d *pdf.Doc, r *model.MonthlyReport, appName string) float64 {
	d.Rect(0, 0, pdf.PageW, 104, ink)
	d.Rect(0, 104, pdf.PageW, 4, accent)
	d.Text(marginX, 44, pdf.Bold, 13, accent, appName)
	d.Text(marginX, 74, pdf.Bold, 24, white, "Laporan "+r.Title)
	d.TextRight(pdf.PageW-marginX, 44, pdf.Regular, 10, white, pdf.Fit(pdf.Regular, 10, 240, r.Owner))
	gen := r.GeneratedAt
	d.TextRight(pdf.PageW-marginX, 74, pdf.Regular, 9, pdf.Hex("#c9d4cf"),
		fmt.Sprintf("Dibuat %d %s %d, %02d.%02d", gen.Day(), monthsShort[gen.Month()-1], gen.Year(), gen.Hour(), gen.Minute()))
	return 134
}

func statTiles(d *pdf.Doc, r *model.MonthlyReport, y float64) float64 {
	net := r.Totals.Income - r.Totals.Expense
	netColor := income
	if net < 0 {
		netColor = expense
	}
	tiles := []struct {
		label string
		value string
		color pdf.RGB
	}{
		{"Pemasukan", Rupiah(r.Totals.Income), income},
		{"Pengeluaran", Rupiah(r.Totals.Expense), expense},
		{"Selisih", Rupiah(net), netColor},
	}
	const gap = 10.0
	w := (pdf.PageW - 2*marginX - 2*gap) / 3
	for i, t := range tiles {
		x := marginX + float64(i)*(w+gap)
		d.Rect(x, y, w, 58, paper)
		d.Rect(x, y, 3, 58, t.color)
		d.Text(x+14, y+21, pdf.Regular, 9, muted, t.label)
		d.Text(x+14, y+43, pdf.Bold, 15, ink, t.value)
	}
	return y + 84
}

func sectionTitle(d *pdf.Doc, title string, y float64) float64 {
	d.Text(marginX, y, pdf.Bold, 12, ink, title)
	return y + 10
}

func walletTable(d *pdf.Doc, r *model.MonthlyReport, y float64) float64 {
	if len(r.Wallets) == 0 {
		return y
	}
	y = sectionTitle(d, "Ringkasan dompet", y)
	right := pdf.PageW - marginX
	cols := []float64{right - 285, right - 195, right - 100, right}
	y += 12
	d.Text(marginX, y, pdf.Bold, 8.5, muted, "Dompet")
	d.TextRight(cols[0], y, pdf.Bold, 8.5, muted, "Masuk")
	d.TextRight(cols[1], y, pdf.Bold, 8.5, muted, "Keluar")
	d.TextRight(cols[2], y, pdf.Bold, 8.5, muted, "Transfer bersih")
	d.TextRight(cols[3], y, pdf.Bold, 8.5, muted, "Saldo saat ini")
	y += 6
	d.Line(marginX, y, right, y, 0.75, hair)
	for _, w := range r.Wallets {
		y += 16
		d.Text(marginX, y, pdf.Regular, 9.5, ink, pdf.Fit(pdf.Regular, 9.5, cols[0]-marginX-80, w.Name))
		d.TextRight(cols[0], y, pdf.Regular, 9.5, ink, Rupiah(w.Income))
		d.TextRight(cols[1], y, pdf.Regular, 9.5, ink, Rupiah(w.Expense))
		moved := "-"
		if net := w.TransferIn - w.TransferOut; net > 0 {
			moved = "+ " + Rupiah(net)
		} else if net < 0 {
			moved = "- " + Rupiah(-net)
		}
		d.TextRight(cols[2], y, pdf.Regular, 9.5, muted, moved)
		d.TextRight(cols[3], y, pdf.Bold, 9.5, ink, Rupiah(w.Balance))
		y += 5
		d.Line(marginX, y, right, y, 0.5, hair)
	}
	return y + 28
}

// foldSlices keeps the top five categories and folds the rest into "Lainnya" so
// the pie stays readable. The table beside it still lists every category.
func foldSlices(slices []model.CategorySlice) []model.CategorySlice {
	if len(slices) <= 6 {
		return slices
	}
	out := append([]model.CategorySlice{}, slices[:5]...)
	other := model.CategorySlice{Name: "Kategori lain", Color: "c0"}
	for _, s := range slices[5:] {
		other.Total += s.Total
	}
	return append(out, other)
}

func breakdown(d *pdf.Doc, title string, slices []model.CategorySlice, total int64, y float64) float64 {
	rowH := 17.0
	height := math.Max(130, float64(len(slices))*rowH+10)
	if y+height+24 > bottomY {
		d.AddPage()
		y = 56
	}
	y = sectionTitle(d, title, y)
	if len(slices) == 0 || total == 0 {
		d.Text(marginX, y+12, pdf.Regular, 9.5, muted, "Belum ada data pada bulan ini.")
		return y + 38
	}

	const radius = 56.0
	cx, cy := marginX+radius+4, y+radius+10
	angle := 0.0
	folded := foldSlices(slices)
	for _, s := range folded {
		sweep := float64(s.Total) / float64(total) * 2 * math.Pi
		d.Sector(cx, cy, radius, angle, angle+sweep, slotColors[s.Color], white)
		angle += sweep
	}

	tableX := marginX + 2*radius + 32
	right := pdf.PageW - marginX
	rowY := y + 18
	for _, s := range slices {
		d.Rect(tableX, rowY-7.5, 8, 8, slotColors[s.Color])
		d.Text(tableX+14, rowY, pdf.Regular, 9.5, ink, pdf.Fit(pdf.Regular, 9.5, 150, s.Name))
		d.TextRight(right-90, rowY, pdf.Regular, 9, muted, fmt.Sprintf("%.1f%%", float64(s.Total)/float64(total)*100))
		d.TextRight(right, rowY, pdf.Bold, 9.5, ink, Rupiah(s.Total))
		rowY += rowH
	}
	return y + height + 18
}

func txTableHeader(d *pdf.Doc, y float64) float64 {
	right := pdf.PageW - marginX
	d.Text(marginX, y, pdf.Bold, 8.5, muted, "Tanggal")
	d.Text(marginX+50, y, pdf.Bold, 8.5, muted, "Kegiatan")
	d.Text(marginX+250, y, pdf.Bold, 8.5, muted, "Kategori")
	d.Text(marginX+345, y, pdf.Bold, 8.5, muted, "Dompet")
	d.TextRight(right, y, pdf.Bold, 8.5, muted, "Nominal")
	y += 6
	d.Line(marginX, y, right, y, 0.75, hair)
	return y
}

func transactionTable(d *pdf.Doc, r *model.MonthlyReport, y float64) {
	if y+70 > bottomY {
		d.AddPage()
		y = 56
	}
	y = sectionTitle(d, fmt.Sprintf("Semua transaksi (%d)", len(r.Transactions)), y)
	if len(r.Transactions) == 0 {
		d.Text(marginX, y+12, pdf.Regular, 9.5, muted, "Belum ada transaksi pada bulan ini.")
		return
	}
	y = txTableHeader(d, y+12)
	right := pdf.PageW - marginX
	for _, t := range r.Transactions {
		if y+20 > bottomY {
			d.AddPage()
			y = txTableHeader(d, 56)
		}
		y += 15
		d.Text(marginX, y, pdf.Regular, 9, muted, shortDate(t.OccurredOn))
		name, category := t.Name, t.CategoryName
		amount, color := "- "+Rupiah(t.Amount), expense
		switch t.Type {
		case model.TxIncome:
			amount, color = "+ "+Rupiah(t.Amount), income
		case model.TxTransferOut:
			// Money changing wallets is neither earned nor spent: neutral ink,
			// no sign. Its fee is listed as its own expense row.
			name, category = "Transfer ke "+t.CounterpartName, "Transfer"
			if t.Name != "" && t.Name != "Transfer" {
				name += " (" + t.Name + ")"
			}
			amount, color = Rupiah(t.Amount), muted
		}
		d.Text(marginX+50, y, pdf.Regular, 9, ink, pdf.Fit(pdf.Regular, 9, 192, name))
		d.Text(marginX+250, y, pdf.Regular, 9, muted, pdf.Fit(pdf.Regular, 9, 88, category))
		d.Text(marginX+345, y, pdf.Regular, 9, muted, pdf.Fit(pdf.Regular, 9, 72, t.WalletName))
		d.TextRight(right, y, pdf.Bold, 9, color, amount)
		y += 5
		d.Line(marginX, y, right, y, 0.5, hair)
	}
}

func footers(d *pdf.Doc, r *model.MonthlyReport, appName string) {
	total := d.PageCount()
	for i := 1; i <= total; i++ {
		d.SetPage(i)
		d.Text(marginX, pdf.PageH-30, pdf.Regular, 8, muted, appName+" · Laporan "+r.Title+" · "+r.Owner)
		d.TextRight(pdf.PageW-marginX, pdf.PageH-30, pdf.Regular, 8, muted, fmt.Sprintf("Halaman %d dari %d", i, total))
	}
}
