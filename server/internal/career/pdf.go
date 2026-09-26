package career

import (
	"bytes"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	pdfPageW = 595.0
	pdfPageH = 842.0
)

// MarkdownToPDF renders a CV as a compact Times-Roman resume (standard 14 fonts).
// The web UI shows what changed; that note is stripped here so it is not printed.
func MarkdownToPDF(title, markdown string) ([]byte, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "CV"
	}
	spans := parseResume(CVForPDF(markdown))
	if len(spans) == 0 {
		spans = []pdfSpan{{kind: spanName, a: title}}
	}
	lines := measureResume(spans)
	if len(lines) == 0 {
		lines = []drawLine{{text: title, font: fontBold, size: 13, leading: 16, align: alignCenter}}
	}
	pages := paginateResume(lines, preferSinglePage(spans))
	return writeResumePDF(pages)
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

func pdfPageCount(pdf []byte) int {
	return bytes.Count(pdf, []byte("/Type /Page /Parent"))
}

type spanKind int

const (
	spanName spanKind = iota
	spanContact
	spanHeading
	spanRow
	spanStack
	spanSkill
	spanBullet
	spanBody
	spanNote
)

type pdfSpan struct {
	kind spanKind
	a, b string
}

type alignKind int

const (
	alignLeft alignKind = iota
	alignCenter
)

const (
	fontRoman  = 1
	fontBold   = 2
	fontItalic = 3
)

type drawLine struct {
	text       string
	right      string
	runs       []textRun
	font       int
	size       float64
	leading    float64
	align      alignKind
	bullet     bool
	indent     float64
	ruleAfter  bool
	ruleBefore bool
	rulePad    float64
	gapBefore  float64
}

type textRun struct {
	text string
	font int
}

func preferSinglePage(spans []pdfSpan) bool {
	n := 0
	for _, s := range spans {
		if s.kind != spanNote {
			n++
		}
	}
	return n > 0 && n <= 90
}

func parseResume(markdown string) []pdfSpan {
	md := htmlCommentRe.ReplaceAllString(markdown, "")
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	var spans []pdfSpan
	i := 0
	for i < len(lines) && strings.TrimSpace(lines[i]) == "" {
		i++
	}
	if i < len(lines) {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "#") && !strings.HasPrefix(t, "##") {
			spans = append(spans, pdfSpan{kind: spanName, a: strings.TrimSpace(strings.TrimLeft(t, "#"))})
			i++
		} else if looksNameLine(lines[i]) {
			spans = append(spans, pdfSpan{kind: spanName, a: strings.TrimSpace(lines[i])})
			i++
		}
	}
	if i < len(lines) && looksContactLine(lines[i]) {
		spans = append(spans, pdfSpan{kind: spanContact, a: strings.TrimSpace(lines[i])})
		i++
	}
	for i < len(lines) {
		raw := strings.TrimRight(lines[i], " \t")
		t := strings.TrimSpace(raw)
		if t == "" {
			i++
			continue
		}
		if strings.HasPrefix(t, "Tailored for ") {
			spans = append(spans, pdfSpan{kind: spanNote, a: t})
			i++
			continue
		}
		if _, ok := headingLine(raw); ok {
			label := strings.TrimSpace(strings.TrimLeft(t, "#"))
			spans = append(spans, pdfSpan{kind: spanHeading, a: label})
			i++
			continue
		}
		if left, right, ok := splitFlushRight(raw); ok {
			spans = append(spans, pdfSpan{kind: spanRow, a: left, b: right})
			i++
			continue
		}
		if isBullet(t) {
			text := bulletText(t)
			i++
			text, i = mergeWrapped(text, lines, i)
			spans = append(spans, pdfSpan{kind: spanBullet, a: text})
			continue
		}
		text := t
		i++
		text, i = mergeWrapped(text, lines, i)
		kind := spanBody
		if looksTechStack(text) && len(spans) > 0 {
			prev := spans[len(spans)-1].kind
			if prev == spanRow || prev == spanStack {
				kind = spanStack
			}
		}
		if kind == spanBody && looksSkillLine(text) {
			kind = spanSkill
		}
		spans = append(spans, pdfSpan{kind: kind, a: text})
	}
	return spans
}

func looksTechStack(s string) bool {
	if strings.Count(s, ",") < 2 {
		return false
	}
	if strings.Contains(s, ". ") {
		return false
	}
	return utf8.RuneCountInString(s) < 240
}

func looksSkillLine(s string) bool {
	i := strings.Index(s, ":")
	if i < 3 || i > 32 {
		return false
	}
	label := s[:i]
	if strings.ContainsAny(label, ",|@") {
		return false
	}
	return !isBullet(s)
}

func firstWord(s string) string {
	s = strings.TrimSpace(s)
	for i, r := range s {
		if r == ',' || r == ';' || r == '.' || r == ' ' {
			return s[:i]
		}
	}
	return s
}

var hyphenSuffixRe = regexp.MustCompile(`^(ing|tion|tation|ment|ness|able|ibility|ation|ed|ly|ers?|als?|uals?|ied|ings)`)

func isHyphenationSuffix(word string) bool {
	return hyphenSuffixRe.MatchString(strings.ToLower(word))
}

func looksNameLine(raw string) bool {
	t := strings.TrimSpace(raw)
	if t == "" || strings.Contains(t, "@") || strings.Contains(t, "|") {
		return false
	}
	if _, ok := headingLine(raw); ok {
		return false
	}
	words := strings.Fields(t)
	if len(words) == 0 || len(words) > 6 || len(t) > 60 {
		return false
	}
	lead := len(raw) - len(strings.TrimLeft(raw, " "))
	return lead >= 8 || len(words) <= 4
}

func looksContactLine(raw string) bool {
	t := strings.ToLower(strings.TrimSpace(raw))
	if t == "" {
		return false
	}
	if strings.Contains(t, "@") || strings.Contains(t, "github") || strings.Contains(t, "linkedin") {
		return true
	}
	if strings.ContainsAny(t, "|") && digitRun(t) >= 8 {
		return true
	}
	return false
}

func digitRun(s string) int {
	best, cur := 0, 0
	for _, r := range s {
		if unicode.IsDigit(r) {
			cur++
			if cur > best {
				best = cur
			}
		} else {
			cur = 0
		}
	}
	return best
}

var (
	yearRe     = regexp.MustCompile(`\b(?:19|20)\d{2}\b`)
	rightColRe = regexp.MustCompile(`(?i)^(github|gitlab|portfolio|present)$`)
)

func splitFlushRight(raw string) (left, right string, ok bool) {
	s := strings.TrimRight(raw, " \t")
	idx := strings.LastIndex(s, "  ")
	if idx < 0 {
		return "", "", false
	}
	start := idx
	for start > 0 && s[start-1] == ' ' {
		start--
	}
	left = strings.TrimSpace(s[:start])
	right = strings.TrimSpace(s[idx:])
	if left == "" || right == "" || len(right) > 42 {
		return "", "", false
	}
	if !looksRightCol(right) {
		return "", "", false
	}
	return left, right, true
}

func looksRightCol(s string) bool {
	if rightColRe.MatchString(strings.TrimSpace(s)) {
		return true
	}
	return yearRe.MatchString(s)
}

func mergeWrapped(text string, lines []string, i int) (string, int) {
	for i < len(lines) {
		raw := strings.TrimRight(lines[i], " \t")
		nt := strings.TrimSpace(raw)
		if nt == "" || isBullet(nt) || strings.HasPrefix(nt, "Tailored for ") {
			break
		}
		if _, ok := headingLine(raw); ok {
			break
		}
		if _, _, ok := splitFlushRight(raw); ok {
			break
		}
		indented := len(raw) > 0 && (raw[0] == ' ' || raw[0] == '\t')
		hyphen := strings.HasSuffix(text, "-")
		nextLower := false
		if r, _ := utf8.DecodeRuneInString(nt); unicode.IsLower(r) {
			nextLower = true
		}
		if hyphen && nextLower {
			first := firstWord(nt)
			if isHyphenationSuffix(first) {
				text = strings.TrimSuffix(text, "-") + nt
			} else {
				text = text + nt
			}
			i++
			continue
		}
		if indented || nextLower {
			text = text + " " + nt
			i++
			continue
		}
		// pdftotext leftover: "Docker" on its own after a long stack line, or
		// an achievements line that continued after a wrap.
		prevLong := utf8.RuneCountInString(text) >= 90
		nextShort := utf8.RuneCountInString(nt) <= 48
		if prevLong && nextShort && !strings.Contains(nt, ":") && !looksRightCol(nt) {
			text = text + " " + nt
			i++
			continue
		}
		if strings.Contains(text, "|") && (strings.Contains(nt, "|") || strings.HasPrefix(nt, "Winner") || strings.HasPrefix(nt, "Ideathon")) {
			text = text + " " + nt
			i++
			continue
		}
		break
	}
	return text, i
}

func measureResume(spans []pdfSpan) []drawLine {
	const (
		bodySize = 8.7
		lead     = 10.35
		usable   = pdfPageW - 64.0
	)
	terms := collectHighlightTerms(spans)
	var out []drawLine
	for _, sp := range spans {
		switch sp.kind {
		case spanName:
			out = append(out, drawLine{text: sp.a, font: fontBold, size: 12.5, leading: 15, align: alignCenter})
		case spanContact:
			out = append(out, drawLine{text: sp.a, font: fontRoman, size: 8.7, leading: 11, align: alignCenter})
		case spanHeading:
			out = append(out, drawLine{text: sp.a, font: fontBold, size: 9.5, leading: 12, ruleAfter: true, rulePad: 3.5, gapBefore: 4})
		case spanRow:
			out = append(out, drawLine{text: sp.a, right: sp.b, font: fontBold, size: bodySize, leading: lead})
		case spanStack:
			chunks := wrapToWidth(sp.a, bodySize, usable)
			for _, c := range chunks {
				out = append(out, drawLine{text: c, font: fontItalic, size: bodySize, leading: lead})
			}
		case spanSkill:
			label, rest, ok := strings.Cut(sp.a, ":")
			if !ok {
				out = append(out, drawLine{text: sp.a, font: fontRoman, size: bodySize, leading: lead})
				break
			}
			out = append(out, drawLine{
				runs: []textRun{
					{text: strings.TrimSpace(label) + ": ", font: fontBold},
					{text: strings.TrimSpace(rest), font: fontRoman},
				},
				size: bodySize, leading: lead,
			})
		case spanBullet:
			chunks := wrapToWidth(sp.a, bodySize, usable-14)
			for i, c := range chunks {
				out = append(out, drawLine{
					runs:    boldTerms(c, terms),
					font:    fontRoman,
					size:    bodySize,
					leading: lead,
					bullet:  i == 0,
					indent:  10,
				})
			}
		case spanNote:
			continue
		default:
			for _, c := range wrapToWidth(sp.a, bodySize, usable) {
				out = append(out, drawLine{runs: boldTerms(c, terms), font: fontRoman, size: bodySize, leading: lead})
			}
		}
	}
	return out
}

func collectHighlightTerms(spans []pdfSpan) []string {
	seen := map[string]bool{}
	var terms []string
	add := func(raw string) {
		for _, p := range strings.Split(raw, ",") {
			p = strings.TrimSpace(p)
			p = strings.Trim(p, ";")
			if i := strings.Index(p, ":"); i >= 0 {
				p = strings.TrimSpace(p[i+1:])
			}
			if utf8.RuneCountInString(p) < 3 || utf8.RuneCountInString(p) > 40 {
				continue
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			terms = append(terms, p)
		}
	}
	for _, sp := range spans {
		switch sp.kind {
		case spanStack, spanSkill:
			add(sp.a)
		}
	}
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	return terms
}

func boldTerms(s string, terms []string) []textRun {
	if s == "" {
		return nil
	}
	if len(terms) == 0 {
		return []textRun{{text: s, font: fontRoman}}
	}
	type mark struct{ start, end int }
	var marks []mark
	for _, term := range terms {
		if term == "" {
			continue
		}
		from := 0
		for {
			i := strings.Index(s[from:], term)
			if i < 0 {
				break
			}
			i += from
			if !termBoundary(s, i, i+len(term)) {
				from = i + 1
				continue
			}
			overlap := false
			for _, m := range marks {
				if i < m.end && i+len(term) > m.start {
					overlap = true
					break
				}
			}
			if !overlap {
				marks = append(marks, mark{i, i + len(term)})
			}
			from = i + len(term)
		}
	}
	if len(marks) == 0 {
		return []textRun{{text: s, font: fontRoman}}
	}
	sort.Slice(marks, func(i, j int) bool { return marks[i].start < marks[j].start })
	var runs []textRun
	cur := 0
	for _, m := range marks {
		if m.start > cur {
			runs = append(runs, textRun{text: s[cur:m.start], font: fontRoman})
		}
		runs = append(runs, textRun{text: s[m.start:m.end], font: fontBold})
		cur = m.end
	}
	if cur < len(s) {
		runs = append(runs, textRun{text: s[cur:], font: fontRoman})
	}
	return runs
}

func termBoundary(s string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:start])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func wrapToWidth(s string, size, maxW float64) []string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return nil
	}
	words := strings.Fields(s)
	var out []string
	var cur strings.Builder
	for _, w := range words {
		cand := w
		if cur.Len() > 0 {
			cand = cur.String() + " " + w
		}
		if timesWidth(cand, size) <= maxW || cur.Len() == 0 {
			if cur.Len() > 0 {
				cur.WriteByte(' ')
			}
			cur.WriteString(w)
			continue
		}
		out = append(out, cur.String())
		cur.Reset()
		cur.WriteString(w)
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

func paginateResume(lines []drawLine, single bool) [][]drawLine {
	const margin = 32.0
	usable := pdfPageH - 2*margin
	if single {
		if fitted, ok := fitOnePage(lines, usable); ok {
			return [][]drawLine{fitted}
		}
	}
	return splitPages(lines, usable)
}

func splitPages(lines []drawLine, usable float64) [][]drawLine {
	if resumeHeight(lines) <= usable {
		return [][]drawLine{lines}
	}
	var pages [][]drawLine
	var cur []drawLine
	h := 0.0
	for _, ln := range lines {
		need := ln.gapBefore + ln.leading + ln.rulePad
		if len(cur) > 0 && h+need > usable {
			pages = append(pages, cur)
			cur = nil
			h = 0
		}
		cur = append(cur, ln)
		h += need
	}
	if len(cur) > 0 {
		pages = append(pages, cur)
	}
	if len(pages) == 0 {
		pages = [][]drawLine{lines}
	}
	return pages
}

// fitOnePage shrinks leading to keep a typical resume on A4. ok is false when
// even 72% still overflows — caller must split pages instead of clipping.
func fitOnePage(lines []drawLine, usable float64) ([]drawLine, bool) {
	if resumeHeight(lines) <= usable {
		return lines, true
	}
	for scale := 0.98; scale >= 0.72; scale -= 0.02 {
		cand := make([]drawLine, len(lines))
		copy(cand, lines)
		for i := range cand {
			cand[i].size *= scale
			cand[i].leading *= scale
			cand[i].gapBefore *= scale
			cand[i].rulePad *= scale
		}
		if resumeHeight(cand) <= usable {
			return cand, true
		}
	}
	return lines, false
}

func resumeHeight(lines []drawLine) float64 {
	h := 0.0
	for _, ln := range lines {
		h += ln.gapBefore + ln.leading + ln.rulePad
	}
	return h
}

func writeResumePDF(pages [][]drawLine) ([]byte, error) {
	const margin = 32.0
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

	type pagePair struct {
		pageID, contentID int
		stream            string
	}
	kids := make([]string, 0, len(pages))
	pageObj := 6
	pairs := make([]pagePair, 0, len(pages))
	for _, page := range pages {
		stream := renderPage(page, margin)
		contentID := pageObj + 1
		pairs = append(pairs, pagePair{pageID: pageObj, contentID: contentID, stream: stream})
		kids = append(kids, fmt.Sprintf("%d 0 R", pageObj))
		pageObj += 2
	}

	writeObj("1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n")
	writeObj(fmt.Sprintf("2 0 obj\n<< /Type /Pages /Kids [%s] /Count %d >>\nendobj\n", strings.Join(kids, " "), len(pages)))
	writeObj("3 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Times-Roman /Encoding /WinAnsiEncoding >>\nendobj\n")
	writeObj("4 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Times-Bold /Encoding /WinAnsiEncoding >>\nendobj\n")
	writeObj("5 0 obj\n<< /Type /Font /Subtype /Type1 /BaseFont /Times-Italic /Encoding /WinAnsiEncoding >>\nendobj\n")
	for _, p := range pairs {
		writeObj(fmt.Sprintf(
			"%d 0 obj\n<< /Type /Page /Parent 2 0 R /MediaBox [0 0 %.0f %.0f] /Contents %d 0 R /Resources << /Font << /F1 3 0 R /F2 4 0 R /F3 5 0 R >> >> >>\nendobj\n",
			p.pageID, pdfPageW, pdfPageH, p.contentID,
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

func renderPage(page []drawLine, margin float64) string {
	var b strings.Builder
	y := pdfPageH - margin
	inText := false
	endText := func() {
		if inText {
			b.WriteString("ET\n")
			inText = false
		}
	}
	beginText := func() {
		if !inText {
			b.WriteString("BT\n")
			inText = true
		}
	}
	for _, ln := range page {
		y -= ln.gapBefore
		if ln.ruleBefore {
			y -= 2
			endText()
			fmt.Fprintf(&b, "0.5 w %.1f %.1f m %.1f %.1f l S\n", margin, y+4, pdfPageW-margin, y+4)
		}
		y -= ln.leading
		beginText()
		font := ln.font
		if font == 0 {
			font = fontRoman
		}
		switch {
		case ln.right != "":
			fmt.Fprintf(&b, "/F%d %.2f Tf\n1 0 0 1 %.1f %.1f Tm\n%s Tj\n", font, ln.size, margin, y, pdfLiteral(pdfWinAnsi(ln.text)))
			rw := timesWidth(ln.right, ln.size)
			fmt.Fprintf(&b, "/F1 %.2f Tf\n1 0 0 1 %.1f %.1f Tm\n%s Tj\n", ln.size, pdfPageW-margin-rw, y, pdfLiteral(pdfWinAnsi(ln.right)))
		case ln.align == alignCenter:
			tw := runWidth(ln.text, ln.size, font)
			x := (pdfPageW - tw) / 2
			if x < margin {
				x = margin
			}
			fmt.Fprintf(&b, "/F%d %.2f Tf\n1 0 0 1 %.1f %.1f Tm\n%s Tj\n", font, ln.size, x, y, pdfLiteral(pdfWinAnsi(ln.text)))
		default:
			x := margin
			if ln.bullet {
				fmt.Fprintf(&b, "/F1 %.2f Tf\n1 0 0 1 %.1f %.1f Tm\n%s Tj\n", ln.size, margin, y, pdfLiteral(pdfWinAnsi("•")))
				x = margin + ln.indent
				if ln.indent == 0 {
					x = margin + 10
				}
			} else if ln.indent > 0 {
				x = margin + ln.indent
			}
			drawRuns(&b, ln, font, x, y)
		}
		if ln.ruleAfter {
			endText()
			fmt.Fprintf(&b, "0.55 w %.1f %.1f m %.1f %.1f l S\n", margin, y-2, pdfPageW-margin, y-2)
			if ln.rulePad > 0 {
				y -= ln.rulePad
			} else {
				y -= 3.5
			}
		}
	}
	endText()
	return b.String()
}

func drawRuns(b *strings.Builder, ln drawLine, fallback int, x, y float64) {
	runs := ln.runs
	if len(runs) == 0 {
		runs = []textRun{{text: ln.text, font: fallback}}
	}
	for _, r := range runs {
		font := r.font
		if font == 0 {
			font = fallback
		}
		fmt.Fprintf(b, "/F%d %.2f Tf\n1 0 0 1 %.1f %.1f Tm\n%s Tj\n", font, ln.size, x, y, pdfLiteral(pdfWinAnsi(r.text)))
		x += runWidth(r.text, ln.size, font)
	}
}

func runWidth(s string, size float64, font int) float64 {
	w := timesWidth(s, size)
	if font == fontBold {
		return w * 1.06
	}
	return w
}

func pdfLiteral(s string) string {
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
		if r <= 127 {
			b.WriteByte(byte(r))
			continue
		}
		if m, ok := winAnsiMap[r]; ok {
			b.WriteByte(m)
			continue
		}
		if r <= 255 {
			b.WriteByte(byte(r))
			continue
		}
		b.WriteByte(' ')
	}
	return b.String()
}

var winAnsiMap = map[rune]byte{
	0x2013: 150, // en dash
	0x2014: 151, // em dash
	0x2018: 145, // ‘
	0x2019: 146, // ’
	0x201C: 147, // “
	0x201D: 148, // ”
	0x2022: 149, // •
	0x2026: 133, // …
	0x00A0: 32,
	0x00D7: 215, // ×
	0x2212: 45,  // minus
	0x00B7: 183,
	0xFB01: 'f', // ﬁ ligature → f (next char often missing; close enough)
	0xFB02: 'f',
}

func timesWidth(s string, size float64) float64 {
	w := 0
	for _, r := range s {
		w += timesGlyph(r)
	}
	return float64(w) * size / 1000.0
}

func timesGlyph(r rune) int {
	if r == 0x2013 || r == 0x2014 {
		return 500
	}
	if r == 0x2022 {
		return 350
	}
	if r > 127 {
		if r <= 255 {
			return 500
		}
		return 500
	}
	if r < 32 {
		return 0
	}
	if int(r) >= 32 && int(r) < 32+len(timesRomanWidths) {
		return timesRomanWidths[r-32]
	}
	return 500
}

// Times-Roman AFM widths, ASCII 32–126, thousandths of an em.
var timesRomanWidths = []int{
	250, 333, 408, 500, 500, 833, 778, 180, 333, 333, 500, 564, 250, 333, 250, 278,
	500, 500, 500, 500, 500, 500, 500, 500, 500, 500, 278, 278, 564, 564, 564, 444, 921,
	722, 667, 667, 722, 611, 556, 722, 722, 333, 389, 722, 611, 889, 722, 722, 556, 722, 667, 556, 611, 722, 722, 944, 722, 722, 611,
	333, 278, 333, 469, 500, 333,
	444, 500, 444, 500, 444, 333, 500, 500, 278, 278, 500, 278, 778, 500, 500, 500, 500, 333, 389, 278, 500, 500, 722, 500, 500, 444,
	480, 200, 480, 541,
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
