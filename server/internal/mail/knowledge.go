package mail

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// MaxKnowledgeURLs is the cap on pinned knowledge pages per mailbox.
const MaxKnowledgeURLs = 20

// ErrInvalidKnowledgeURL is returned when PATCH knowledge_urls contains a
// disallowed scheme (javascript:, data:) or a non-http(s) URL.
var ErrInvalidKnowledgeURL = errors.New("mail: invalid knowledge url")

// SanitizeKnowledgeURLs trims entries, drops blanks, keeps http(s) only, and
// caps the list. javascript: and data: are rejected rather than dropped so the
// operator can see the mistake.
func SanitizeKnowledgeURLs(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := make(map[string]struct{}, len(in))
	for _, raw := range in {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: %q", ErrInvalidKnowledgeURL, raw)
		}
		scheme := strings.ToLower(u.Scheme)
		if scheme == "javascript" || scheme == "data" {
			return nil, fmt.Errorf("%w: %s: URLs are not allowed", ErrInvalidKnowledgeURL, scheme)
		}
		if scheme != "http" && scheme != "https" {
			return nil, fmt.Errorf("%w: %q must be http or https", ErrInvalidKnowledgeURL, raw)
		}
		if u.Host == "" {
			return nil, fmt.Errorf("%w: %q has no host", ErrInvalidKnowledgeURL, raw)
		}
		canonical := u.String()
		if _, dup := seen[canonical]; dup {
			continue
		}
		seen[canonical] = struct{}{}
		out = append(out, canonical)
		if len(out) >= MaxKnowledgeURLs {
			break
		}
	}
	return out, nil
}
