package marketingmanager

import (
	"net/mail"
	"strings"
)

// ValidEmail reports whether s is one plain address (no display name) with a
// dotted domain, and returns it trimmed and lower-cased.
func ValidEmail(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > 254 {
		return "", false
	}
	a, err := mail.ParseAddress(s)
	if err != nil || a.Name != "" || a.Address != s {
		return "", false
	}
	at := strings.LastIndexByte(s, '@')
	if at < 1 {
		return "", false
	}
	domain := s[at+1:]
	if !strings.Contains(domain, ".") || strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") || strings.Contains(domain, "..") {
		return "", false
	}
	return s, true
}
