// Package reportpdf builds simple multi-page A4 PDF reports (Times standard fonts).
package reportpdf

import (
	"fmt"
	"strings"
	"unicode"
)

const (
	pageW  = 595.0
	pageH  = 842.0
	margin = 48.0
)

// Doc is a printable security / lab report.
type Doc struct {
	Title    string
	Subtitle string
	Meta     []Meta
	Sections []Section
}

// Meta is a label/value pair shown under the title.
type Meta struct {
	Label, Value string
}

// Section is a headed block of wrapped text lines.
type Section struct {
	Heading string
	Lines   []string
}

// Render writes Doc as PDF 1.4 bytes.
func Render(d Doc) ([]byte, error) {
	return writePDF(paginate(layout(d)))
}

// Filename builds a safe download name from parts.
func Filename(parts ...string) string {
	var out []string
	for _, p := range parts {
		if s := slug(p); s != "" {
			out = append(out, s)
		}
	}
	if len(out) == 0 {
		return "report.pdf"
	}
	return strings.Join(out, "-") + ".pdf"
}

type drawLine struct {
	text    string
	font    int // 1 roman 2 bold
	size    float64
	leading float64
	gap     float64
}

func layout(d Doc) []drawLine {
	var out []drawLine
	title := strings.TrimSpace(d.Title)
	if title == "" {
		title = "Report"
	}
	out = append(out, drawLine{text: title, font: 2, size: 16, leading: 20, gap: 4})
	if s := strings.TrimSpace(d.Subtitle); s != "" {
		out = append(out, drawLine{text: s, font: 1, size: 10, leading: 13, gap: 2})
	}
	out = append(out, drawLine{text: "", font: 1, size: 8, leading: 6, gap: 2})
	for _, m := range d.Meta {
		label := strings.TrimSpace(m.Label)
		val := strings.TrimSpace(m.Value)
		if label == "" && val == "" {
			continue
		}
		out = append(out, drawLine{
			text: fmt.Sprintf("%s: %s", label, val), font: 1, size: 9, leading: 12, gap: 0,
		})
	}
	out = append(out, drawLine{text: "", font: 1, size: 8, leading: 10, gap: 0})
	for _, sec := range d.Sections {
		h := strings.TrimSpace(sec.Heading)
		if h != "" {
			out = append(out, drawLine{text: h, font: 2, size: 12, leading: 16, gap: 6})
		}
		for _, raw := range sec.Lines {
			for _, wrapped := range wrap(strings.TrimSpace(raw), 92) {
				out = append(out, drawLine{text: wrapped, font: 1, size: 9, leading: 12, gap: 0})
			}
		}
		out = append(out, drawLine{text: "", font: 1, size: 8, leading: 8, gap: 0})
	}
	return out
}

func wrap(s string, max int) []string {
	if s == "" {
		return []string{""}
	}
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := words[0]
	for _, w := range words[1:] {
		if len(cur)+1+len(w) <= max {
			cur += " " + w
			continue
		}
		lines = append(lines, cur)
		cur = w
	}
	return append(lines, cur)
}

func paginate(lines []drawLine) [][]drawLine {
	const usable = pageH - 2*margin
	var pages [][]drawLine
	var cur []drawLine
	y := 0.0
	for _, ln := range lines {
		need := ln.leading + ln.gap
		if len(cur) > 0 && y+need > usable {
			pages = append(pages, cur)
			cur = nil
			y = 0
		}
		cur = append(cur, ln)
		y += need
	}
	if len(cur) > 0 {
		pages = append(pages, cur)
	}
	if len(pages) == 0 {
		pages = [][]drawLine{{{text: "Empty report", font: 1, size: 10, leading: 14}}}
	}
	return pages
}

func writePDF(pages [][]drawLine) ([]byte, error) {
	n := len(pages)
	// Objects: 1 catalog, 2 pages, 3 F1, 4 F2, then pairs (content, page) starting at 5.
	type obj struct {
		n int
		b string
	}
	objs := make([]obj, 0, 4+2*n)
	kids := make([]string, 0, n)
	for i := 0; i < n; i++ {
		pageObj := 5 + i*2 + 1
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
	}
	objs = append(objs,
		obj{1, "<< /Type /Catalog /Pages 2 0 R >>"},
		obj{2, fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", strings.Join(kids, " "), n)},
		obj{3, "<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman >>"},
		obj{4, "<< /Type /Font /Subtype /Type1 /BaseFont /Times-Bold >>"},
	)
	for i, page := range pages {
		contentObj := 5 + i*2
		pageObj := contentObj + 1
		content := pageContent(page)
		objs = append(objs,
			obj{contentObj, fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content)},
			obj{pageObj, fmt.Sprintf(
				"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Contents %d 0 R /Resources << /Font << /F1 3 0 R /F2 4 0 R >> >> >>",
				pageW, pageH, contentObj,
			)},
		)
	}

	var body strings.Builder
	body.WriteString("%PDF-1.4\n")
	offsets := make([]int, len(objs)+1)
	for _, o := range objs {
		offsets[o.n] = body.Len()
		fmt.Fprintf(&body, "%d 0 obj\n%s\nendobj\n", o.n, o.b)
	}
	xrefAt := body.Len()
	fmt.Fprintf(&body, "xref\n0 %d\n", len(objs)+1)
	body.WriteString("0000000000 65535 f \n")
	for i := 1; i <= len(objs); i++ {
		fmt.Fprintf(&body, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xrefAt)
	return []byte(body.String()), nil
}

func pageContent(lines []drawLine) string {
	var b strings.Builder
	b.WriteString("BT\n")
	y := pageH - margin
	for _, ln := range lines {
		y -= ln.gap
		y -= ln.leading
		if y < margin {
			break
		}
		font := 1
		if ln.font == 2 {
			font = 2
		}
		fmt.Fprintf(&b, "/F%d %.2f Tf\n1 0 0 1 %.1f %.1f Tm\n%s Tj\n",
			font, ln.size, margin, y, literal(winAnsi(ln.text)))
	}
	b.WriteString("ET\n")
	return b.String()
}

func literal(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '\\', '(', ')':
			b.WriteByte('\\')
			b.WriteByte(c)
		default:
			if c < 32 || c > 126 {
				fmt.Fprintf(&b, "\\%03o", c)
			} else {
				b.WriteByte(c)
			}
		}
	}
	b.WriteByte(')')
	return b.String()
}

func winAnsi(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteByte(' ')
		case r < 32 || r == 127:
			continue
		case r <= 127:
			b.WriteByte(byte(r))
		case r == 0x2013 || r == 0x2014:
			b.WriteByte('-')
		case r == 0x2018 || r == 0x2019:
			b.WriteByte('\'')
		case r == 0x201C || r == 0x201D:
			b.WriteByte('"')
		case r == 0x2022:
			b.WriteByte('*')
		case r <= 255:
			b.WriteByte(byte(r))
		default:
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func slug(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prev := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prev = false
			continue
		}
		if !prev && b.Len() > 0 {
			b.WriteByte('_')
			prev = true
		}
	}
	return strings.Trim(b.String(), "_")
}
