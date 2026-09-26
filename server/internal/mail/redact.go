package mail

import (
	"errors"
	"regexp"
	"strings"
)

// secretParam matches query/form fields that must never appear in logs or API errors.
var secretParam = regexp.MustCompile(`(?i)(refresh_token|access_token|id_token|client_secret|code)=[^&\s]+`)

// Redact strips token-shaped fields from a string.
func Redact(s string) string {
	s = secretParam.ReplaceAllString(s, "$1=<redacted>")
	s = strings.ReplaceAll(s, "Bearer ", "Bearer <redacted>")
	return s
}

// RedactErr returns an error whose message has been Redact'd. The original
// error is not wrapped, so a logger cannot recover the secret via %v/%+v of a
// chain that still holds the raw text.
func RedactErr(err error) error {
	if err == nil {
		return nil
	}
	return errors.New(Redact(err.Error()))
}
