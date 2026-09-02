package career

import (
	"regexp"
	"strings"
	"unicode"
)

// HeadingOutline is the ordered list of section headings in a CV.
// Markdown headings, known resume section titles, and short ALL-CAPS lines count.
func HeadingOutline(md string) []string {
	var out []string
	for _, line := range strings.Split(stripTailorChrome(md), "\n") {
		if h, ok := headingLine(line); ok {
			out = append(out, h)
		}
	}
	return out
}

// SameOutline reports whether two CVs share the same heading sequence.
// Documents with no headings are treated as matching only if both have none.
func SameOutline(src, dest string) bool {
	a, b := HeadingOutline(src), HeadingOutline(dest)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// KeepLayout is the tailor gate: same sections, same bullet count, no expansion
// into extra pages. A short trailing "Tailored for …" note is ignored.
func KeepLayout(src, dest string) bool {
	src, dest = stripTailorChrome(src), stripTailorChrome(dest)
	if src == "" || dest == "" {
		return false
	}
	if !SameOutline(src, dest) {
		return false
	}
	if countBullets(src) != countBullets(dest) {
		return false
	}
	if contentLines(dest) > contentLines(src)+3 {
		return false
	}
	if utf8Len(dest) > utf8Len(src)*112/100+120 {
		return false
	}
	return true
}

func headingLine(line string) (string, bool) {
	t := strings.TrimSpace(line)
	if t == "" {
		return "", false
	}
	if strings.HasPrefix(t, "#") {
		h := strings.TrimSpace(strings.TrimLeft(t, "#"))
		if h == "" {
			return "", false
		}
		return strings.ToLower(h), true
	}
	if isAllCapsHeading(t) {
		return strings.ToLower(t), true
	}
	if known, ok := knownSection(t); ok {
		return known, true
	}
	return "", false
}

func knownSection(t string) (string, bool) {
	key := strings.ToLower(strings.TrimSpace(t))
	key = strings.TrimRight(key, ":")
	if resumeSections[key] {
		return key, true
	}
	return "", false
}

// resumeSections are Title Case lines from pdftotext -layout (not markdown #).
var resumeSections = map[string]bool{
	"summary": true, "objective": true, "profile": true, "about": true,
	"education": true, "academic": true,
	"experience": true, "work experience": true, "professional experience": true,
	"internship experience": true, "internships": true, "employment": true,
	"projects": true, "selected projects": true, "personal projects": true,
	"skills": true, "skills and competencies": true, "technical skills": true,
	"tools": true, "tools / platforms": true,
	"achievements": true, "awards": true, "honors": true, "honours": true,
	"publications": true, "research": true,
	"certifications": true, "certificates": true,
	"volunteer": true, "volunteering": true, "languages": true,
	"interests": true, "references": true,
}

func isAllCapsHeading(t string) bool {
	if len(t) < 3 || len(t) > 48 {
		return false
	}
	letters := 0
	for _, r := range t {
		if unicode.IsLetter(r) {
			letters++
			if !unicode.IsUpper(r) {
				return false
			}
		} else if r != ' ' && r != '&' && r != '/' && r != '-' {
			return false
		}
	}
	return letters >= 3
}

func unchangedLayoutNote(role, company string) string {
	return visibleTailorNote(role, company, "layout unchanged; no claims were added.")
}

// CVForPDF is the downloadable CV. The "Tailored for …" line is a web note, not page content.
func CVForPDF(markdown string) string {
	return stripTailorChrome(markdown)
}

func visibleTailorNote(role, company, detail string) string {
	r := strings.TrimSpace(role)
	if r == "" {
		r = "this role"
	}
	c := strings.TrimSpace(company)
	if c == "" {
		c = "this company"
	}
	d := strings.TrimSpace(detail)
	d = strings.Trim(d, `"'`)
	if d == "" {
		d = "keywords already on this CV were left in place; layout unchanged."
	}
	if len([]rune(d)) > 220 {
		d = string([]rune(d)[:217]) + "…"
	}
	return "\n\nTailored for " + r + " at " + c + " — " + d
}

var htmlCommentRe = regexp.MustCompile(`(?s)<!--.*?-->`)

func stripTailorChrome(s string) string {
	s = htmlCommentRe.ReplaceAllString(s, "")
	var keep []string
	for _, line := range strings.Split(s, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "Tailored for ") {
			continue
		}
		keep = append(keep, line)
	}
	return strings.TrimSpace(strings.Join(keep, "\n"))
}

func countBullets(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if isBullet(strings.TrimSpace(line)) {
			n++
		}
	}
	return n
}

func contentLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

func utf8Len(s string) int { return len([]rune(s)) }

func isBullet(t string) bool {
	if t == "" {
		return false
	}
	if strings.HasPrefix(t, "•") || strings.HasPrefix(t, "●") || strings.HasPrefix(t, "‣") {
		return true
	}
	if strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") || strings.HasPrefix(t, "– ") || strings.HasPrefix(t, "— ") {
		return true
	}
	return false
}

func bulletText(t string) string {
	t = strings.TrimSpace(t)
	for _, p := range []string{"•", "●", "‣", "- ", "* ", "– ", "— "} {
		if strings.HasPrefix(t, p) {
			return strings.TrimSpace(t[len(p):])
		}
	}
	return t
}
