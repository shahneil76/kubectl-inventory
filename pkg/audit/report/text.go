package report

import (
	"strings"
	"unicode"

	"github.com/go-pdf/fpdf"
)

// pdfPunctuation maps common Unicode punctuation to ASCII for core PDF fonts.
var pdfPunctuation = strings.NewReplacer(
	"\u2014", "-", // em dash
	"\u2013", "-", // en dash
	"\u2212", "-", // minus sign
	"\u00b7", " | ", // middle dot
	"\u2022", "*", // bullet
	"\u2018", "'", // left single quote
	"\u2019", "'", // right single quote
	"\u201c", "\"", // left double quote
	"\u201d", "\"", // right double quote
	"\u2026", "...", // ellipsis
	"\u2192", "->", // right arrow
	"\u2190", "<-", // left arrow
	"\u00a0", " ", // non-breaking space
)

// pdfText normalizes user-facing strings for fpdf core fonts (Latin-1 / CP1252).
func pdfText(s string) string {
	s = pdfPunctuation.Replace(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r == '\n' || r == '\t':
			b.WriteRune(r)
		case r >= 32 && r <= 255:
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteRune(' ')
		}
	}
	return b.String()
}

// pdfCell writes a single line with ASCII-safe text for core PDF fonts.
func pdfCell(pdf *fpdf.Fpdf, w, h float64, txtStr, borderStr string, ln int, alignStr string, fill bool, link int, linkStr string) {
	pdf.CellFormat(w, h, pdfText(txtStr), borderStr, ln, alignStr, fill, link, linkStr)
}

// pdfMultiCell writes wrapped text with ASCII-safe encoding.
func pdfMultiCell(pdf *fpdf.Fpdf, w, h float64, txtStr, borderStr, alignStr string, fill bool) {
	pdf.MultiCell(w, h, pdfText(txtStr), borderStr, alignStr, fill)
}
