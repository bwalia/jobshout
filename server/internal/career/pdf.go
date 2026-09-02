package career

import (
	"bytes"
	"fmt"
	"strings"
	"unicode"
)

// MarkdownToPDF renders CV markdown as a simple multi-page PDF (Helvetica).
func MarkdownToPDF(title, markdown string) ([]byte, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "CV"
	}
	lines := wrapPDFLines(markdown)
	if len(lines) == 0 {
		lines = []pdfLine{{text: title, heading: true}}
	}

	const (
		pageW    = 595.0
		pageH    = 842.0
		margin   = 48.0
		bodySize = 10.0
		headSize = 13.0
		leading  = 14.0
	)
	usable := pageH - 2*margin
	perPage := int(usable / leading)
	if perPage < 1 {
		perPage = 40
	}

	var pages [][]pdfLine
	for i := 0; i < len(lines); i += perPage {
		end := i + perPage
		if end > len(lines) {
			end = len(lines)
		}
		pages = append(pages, lines[i:end])
	}

	var body bytes.Buffer
	body.WriteString("%PDF-1.4\n")
	offsets := []int{0}

	writeObj := func(s string) {
		offsets = append(offsets, body.Len())
		body.WriteString(s)
		if !strings.HasSuffix(s, "\n") {
			body.WriteByte('\n')
		}
	}

	// 1 catalog, 2 pages tree, 3 font, then 2 objects per page (page + content)
	fontObj := 3
	kids := make([]string, 0, len(pages))
	pageObj := 4
	type pagePair struct {
		pageID, contentID int
		stream            string
	}
	pairs := make([]pagePair, 0, len(pages))
	for _, page := range pages {
		var stream strings.Builder
		stream.WriteString("BT\n")
		y := pageH - margin
		for i, ln := range page {
			size := bodySize
			if ln.heading {
				size = headSize
			}
			if i == 0 {
				fmt.Fprintf(&stream, "/F1 %.1f Tf\n%.1f %.1f Td\n", size, margin, y)
			} else {
				prev := bodySize
				if page[i-1].heading {
					prev = headSize
				}
				if size != prev {
					fmt.Fprintf(&stream, "/F1 %.1f Tf\n", size)
				}
				fmt.Fprintf(&stream, "0 -%.1f Td\n", leading)
			}
			fmt.Fprintf(&stream, "(%s) Tj\n", pdfEscape(ln.text))
		}
		stream.WriteString("ET\n")
		contentID := pageObj + 1
		pairs = append(pairs, pagePair{pageID: pageObj, contentID: contentID, stream: stream.String()})
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
		pageObj += 2
	}

	writeObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	writeObj(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), len(pages)))
	writeObj("3 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\nendobj\n")
	_ = fontObj
	for _, p := range pairs {
		writeObj(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Contents %d 0 R /Resources << /Font << /F1 3 0 R >> >> >>\nendobj\n",
			p.pageID, pageW, pageH, p.contentID,
		))
		writeObj(fmt.Sprintf("%d 0 obj\n<< /Length %d >>\nstream\n%s\nendstream\nendobj\n", p.contentID, len(p.stream), p.stream))
	}

	xrefAt := body.Len()
	fmt.Fprintf(&body, "xref\n0 %d\n0000000000 65535 f \n", len(offsets))
	for i := 1; i < len(offsets); i++ {
		fmt.Fprintf(&body, "%010d 00000 n \n", offsets[i])
	}
	fmt.Fprintf(&body, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(offsets), xrefAt)
	return body.Bytes(), nil
}

// PDFFilename is a safe download name, e.g. Ada_Lovelace-Acme-Staff_Engineer.pdf
func PDFFilename(person, company, role string) string {
	parts := []string{}
	for _, s := range []string{person, company, role} {
		if t := pdfSlug(s); t != "" {
			parts = append(parts, t)
		}
	}
	if len(parts) == 0 {
		return "tailored-cv.pdf"
	}
	return strings.Join(parts, "-") + ".pdf"
}

type pdfLine struct {
	text    string
	heading bool
}

func wrapPDFLines(markdown string) []pdfLine {
	var out []pdfLine
	for _, raw := range strings.Split(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n") {
		heading := false
		s := strings.TrimRight(raw, " \t")
		if strings.HasPrefix(s, "#") {
			heading = true
			s = strings.TrimSpace(strings.TrimLeft(s, "#"))
		}
		s = pdfWinAnsi(s)
		if s == "" {
			out = append(out, pdfLine{text: " ", heading: heading})
			continue
		}
		const max = 92
		for _, chunk := range wrapWords(s, max) {
			out = append(out, pdfLine{text: chunk, heading: heading})
			heading = false
		}
	}
	return out
}

func wrapWords(s string, max int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	var out []string
	var cur strings.Builder
	for _, w := range words {
		if cur.Len() == 0 {
			cur.WriteString(w)
			continue
		}
		if cur.Len()+1+len(w) > max {
			out = append(out, cur.String())
			cur.Reset()
			cur.WriteString(w)
			continue
		}
		cur.WriteByte(' ')
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func pdfEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `(`, `\(`)
	s = strings.ReplaceAll(s, `)`, `\)`)
	return s
}

func pdfWinAnsi(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\t' {
			b.WriteByte(' ')
			continue
		}
		if r < 32 || r == 127 {
			continue
		}
		if r <= 255 {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('?')
	}
	return b.String()
}

func pdfSlug(s string) string {
	s = strings.TrimSpace(s)
	var b strings.Builder
	prevUnderscore := false
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			prevUnderscore = false
			continue
		}
		if !prevUnderscore && b.Len() > 0 {
			b.WriteByte('_')
			prevUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}
