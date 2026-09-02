package career

import (
	"strings"
	"unicode"
)

// HeadingOutline is the ordered list of section headings in a CV.
// Markdown headings (# …) and short ALL-CAPS lines both count.
func HeadingOutline(md string) []string {
	var out []string
	for _, line := range strings.Split(md, "\n") {
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
	return "", false
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
	r := strings.TrimSpace(role)
	if r == "" {
		r = "this role"
	}
	c := strings.TrimSpace(company)
	if c == "" {
		c = "this company"
	}
	return "\n\n<!-- keywords highlighted for " + r + " at " + c + " — layout unchanged -->\n"
}
