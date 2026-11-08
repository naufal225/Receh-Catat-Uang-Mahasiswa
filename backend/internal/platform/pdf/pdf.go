// Package pdf is a minimal PDF 1.4 writer: pages, text in the two built-in
// Helvetica fonts, filled shapes and lines. That is all a monthly report needs,
// so it replaces a third-party dependency with one small file. Text is limited
// to the WinAnsi character set; anything else prints as "?".
package pdf

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"math"
	"strings"
)

const (
	PageW = 595.28
	PageH = 841.89
)

type Font string

const (
	Regular Font = "F1"
	Bold    Font = "F2"
)

// RGB is a colour with components from 0 to 1.
type RGB struct{ R, G, B float64 }

// Hex parses "#rrggbb"; a malformed value becomes black.
func Hex(s string) RGB {
	var r, g, b int
	if _, err := fmt.Sscanf(s, "#%02x%02x%02x", &r, &g, &b); err != nil {
		return RGB{}
	}
	return RGB{float64(r) / 255, float64(g) / 255, float64(b) / 255}
}

// Doc is a document under construction. Coordinates use a top-left origin in
// points, which is easier to lay out with than PDF's native bottom-left origin.
type Doc struct {
	title string
	pages []*bytes.Buffer
	cur   *bytes.Buffer
}

func New(title string) *Doc { return &Doc{title: title} }

func (d *Doc) AddPage() {
	d.cur = &bytes.Buffer{}
	d.pages = append(d.pages, d.cur)
}

func (d *Doc) PageCount() int { return len(d.pages) }

// SetPage returns drawing to an earlier page (1-indexed), used to stamp footers.
func (d *Doc) SetPage(n int) { d.cur = d.pages[n-1] }

func flipY(y float64) float64 { return PageH - y }

// Text draws s with its baseline at (x, y).
func (d *Doc) Text(x, y float64, f Font, size float64, c RGB, s string) {
	fmt.Fprintf(d.cur, "BT /%s %.2f Tf %.3f %.3f %.3f rg %.2f %.2f Td <%s> Tj ET\n",
		f, size, c.R, c.G, c.B, x, flipY(y), hexWinAnsi(s))
}

// TextRight draws s so that it ends at x.
func (d *Doc) TextRight(x, y float64, f Font, size float64, c RGB, s string) {
	d.Text(x-Width(f, size, s), y, f, size, c, s)
}

// Fit shortens s with an ellipsis until it fits in maxW points.
func Fit(f Font, size, maxW float64, s string) string {
	if Width(f, size, s) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && Width(f, size, string(r)+"...") > maxW {
		r = r[:len(r)-1]
	}
	return strings.TrimRight(string(r), " ") + "..."
}

func (d *Doc) Rect(x, y, w, h float64, c RGB) {
	fmt.Fprintf(d.cur, "%.3f %.3f %.3f rg %.2f %.2f %.2f %.2f re f\n", c.R, c.G, c.B, x, flipY(y+h), w, h)
}

func (d *Doc) Line(x1, y1, x2, y2, width float64, c RGB) {
	fmt.Fprintf(d.cur, "%.3f %.3f %.3f RG %.2f w %.2f %.2f m %.2f %.2f l S\n", c.R, c.G, c.B, width, x1, flipY(y1), x2, flipY(y2))
}

// Sector fills a pie slice. Angles are in radians, clockwise from 12 o'clock.
// The arc is split into quarter-or-less Bezier segments, and a thin stroke in
// gapColor keeps neighbouring slices visually separate.
func (d *Doc) Sector(cx, cy, radius, start, end float64, fill, gapColor RGB) {
	point := func(a float64) (float64, float64) {
		return cx + radius*math.Sin(a), flipY(cy - radius*math.Cos(a))
	}
	// A single category is a whole disc: no wedge edges, so no seam from the
	// centre to the rim, and fill only.
	full := end-start >= 2*math.Pi-1e-9
	fmt.Fprintf(d.cur, "%.3f %.3f %.3f rg %.3f %.3f %.3f RG 1.5 w\n",
		fill.R, fill.G, fill.B, gapColor.R, gapColor.G, gapColor.B)
	x0, y0 := point(start)
	if full {
		fmt.Fprintf(d.cur, "%.2f %.2f m\n", x0, y0)
	} else {
		fmt.Fprintf(d.cur, "%.2f %.2f m %.2f %.2f l\n", cx, flipY(cy), x0, y0)
	}
	segments := int(math.Ceil((end - start) / (math.Pi / 2)))
	if segments < 1 {
		segments = 1
	}
	step := (end - start) / float64(segments)
	k := 4.0 / 3.0 * math.Tan(step/4) * radius
	for i := range segments {
		a0, a1 := start+float64(i)*step, start+float64(i+1)*step
		xa, ya := point(a0)
		xb, yb := point(a1)
		// Tangent direction of a clockwise arc, in PDF's y-up space.
		c1x, c1y := xa+k*math.Cos(a0), ya-k*math.Sin(a0)
		c2x, c2y := xb-k*math.Cos(a1), yb+k*math.Sin(a1)
		fmt.Fprintf(d.cur, "%.2f %.2f %.2f %.2f %.2f %.2f c\n", c1x, c1y, c2x, c2y, xb, yb)
	}
	if full {
		fmt.Fprint(d.cur, "h f\n")
		return
	}
	fmt.Fprint(d.cur, "h B\n")
}

// Bytes serialises the document.
func (d *Doc) Bytes() ([]byte, error) {
	var out bytes.Buffer
	out.WriteString("%PDF-1.4\n%\xE2\xE3\xCF\xD3\n")
	offsets := []int{0}
	obj := func(body string) {
		offsets = append(offsets, out.Len())
		fmt.Fprintf(&out, "%d 0 obj\n%s\nendobj\n", len(offsets)-1, body)
	}

	// Objects: 1 catalog, 2 page tree, 3-4 fonts, 5 info, then a page and a
	// content stream for each page.
	const firstPageObj = 6
	kids := make([]string, len(d.pages))
	for i := range d.pages {
		kids[i] = fmt.Sprintf("%d 0 R", firstPageObj+2*i)
	}
	obj("<< /Type /Catalog /Pages 2 0 R >>")
	obj(fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), len(d.pages)))
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>")
	obj("<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica-Bold /Encoding /WinAnsiEncoding >>")
	obj(fmt.Sprintf("<< /Title <%s> /Producer (Receh) >>", hexWinAnsi(d.title)))

	for i, page := range d.pages {
		obj(fmt.Sprintf("<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.2f %.2f] /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> /Contents %d 0 R >>",
			PageW, PageH, firstPageObj+2*i+1))
		var z bytes.Buffer
		w := zlib.NewWriter(&z)
		if _, err := w.Write(page.Bytes()); err != nil {
			return nil, err
		}
		if err := w.Close(); err != nil {
			return nil, err
		}
		obj(fmt.Sprintf("<< /Length %d /Filter /FlateDecode >>\nstream\n%s\nendstream", z.Len(), z.String()))
	}

	xref := out.Len()
	fmt.Fprintf(&out, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for _, off := range offsets[1:] {
		fmt.Fprintf(&out, "%010d 00000 n \n", off)
	}
	fmt.Fprintf(&out, "trailer\n<< /Size %d /Root 1 0 R /Info 5 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xref)
	return out.Bytes(), nil
}

// winAnsi maps the characters above Latin-1 that a report is likely to contain.
var winAnsi = map[rune]byte{
	'€': 0x80, '‚': 0x82, '„': 0x84, '…': 0x85, '‘': 0x91, '’': 0x92, '“': 0x93,
	'”': 0x94, '•': 0x95, '–': 0x96, '—': 0x97, '™': 0x99,
}

func toWinAnsi(r rune) byte {
	if r >= 0x20 && r < 0x7F || r >= 0xA0 && r <= 0xFF {
		return byte(r)
	}
	if b, ok := winAnsi[r]; ok {
		return b
	}
	return '?'
}

func hexWinAnsi(s string) string {
	var b strings.Builder
	for _, r := range s {
		fmt.Fprintf(&b, "%02X", toWinAnsi(r))
	}
	return b.String()
}

// Width measures s in points using the standard Helvetica metrics.
func Width(f Font, size float64, s string) float64 {
	table := &helvetica
	if f == Bold {
		table = &helveticaBold
	}
	total := 0
	for _, r := range s {
		c := toWinAnsi(r)
		if c >= 32 && c <= 126 {
			total += table[c-32]
		} else {
			total += 556
		}
	}
	return float64(total) * size / 1000
}

// Glyph widths for ASCII 32-126, from the Adobe core font metrics.
var helvetica = [95]int{
	278, 278, 355, 556, 556, 889, 667, 191, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 278, 278, 584, 584, 584, 556,
	1015, 667, 667, 722, 722, 667, 611, 778, 722, 278, 500, 667, 556, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 278, 278, 278, 469, 556,
	333, 556, 556, 500, 556, 556, 278, 556, 556, 222, 222, 500, 222, 833, 556, 556,
	556, 556, 333, 500, 278, 556, 500, 722, 500, 500, 500, 334, 260, 334, 584,
}

var helveticaBold = [95]int{
	278, 333, 474, 556, 556, 889, 722, 238, 333, 333, 389, 584, 278, 333, 278, 278,
	556, 556, 556, 556, 556, 556, 556, 556, 556, 556, 333, 333, 584, 584, 584, 611,
	975, 722, 722, 722, 722, 667, 611, 778, 722, 278, 556, 722, 611, 833, 722, 778,
	667, 778, 722, 667, 611, 722, 667, 944, 667, 667, 611, 333, 278, 333, 584, 556,
	333, 556, 611, 556, 611, 556, 333, 611, 611, 278, 278, 556, 278, 889, 611, 611,
	611, 611, 389, 556, 333, 611, 556, 778, 556, 556, 500, 389, 280, 389, 584,
}
