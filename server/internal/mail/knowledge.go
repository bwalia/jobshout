package mail

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// MaxKnowledgeURLs is the cap on pinned knowledge pages per mailbox.
const MaxKnowledgeURLs = 20

// MaxInboundURLs is how many links we take from one inbound email.
const MaxInboundURLs = 5

// inboundURLRe finds http(s) URLs in email subject/body. Angle brackets and
// trailing punctuation are stripped after the match.
var inboundURLRe = regexp.MustCompile(`https?://[^\s<>"'()]+`)

var trackingHostFrags = []string{
	"unsubscribe",
	"list-manage.",
	"mailchimp.",
	"sendgrid.net",
	"sparkpostmail.com",
	"click.",
	"track.",
	"email.",
	"cndtrk.",
	"google.com/url",
}

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

// ExtractInboundURLs pulls http(s) links from subject and body, drops
// unsubscribe/tracking hosts, and caps the list. Invalid schemes are skipped
// rather than rejected — inbound mail is untrusted.
func ExtractInboundURLs(subject, body string) []string {
	blob := subject + "\n" + body
	raw := inboundURLRe.FindAllString(blob, MaxInboundURLs*4)
	cleaned := make([]string, 0, len(raw))
	for _, u := range raw {
		u = strings.TrimRight(u, ".,;:!?)]}")
		if isTrackingURL(u) {
			continue
		}
		cleaned = append(cleaned, u)
	}
	out, err := SanitizeKnowledgeURLs(cleaned)
	if err != nil {
		// Drop bad entries one by one instead of failing the whole email.
		out = nil
		for _, u := range cleaned {
			one, e := SanitizeKnowledgeURLs([]string{u})
			if e != nil {
				continue
			}
			out = append(out, one...)
		}
	}
	if len(out) > MaxInboundURLs {
		out = out[:MaxInboundURLs]
	}
	return out
}

// MergeKnowledgeURLs puts playbook pages first, then inbound links, deduped.
func MergeKnowledgeURLs(playbook, inbound []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(playbook)+len(inbound))
	for _, u := range append(append([]string{}, playbook...), inbound...) {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
		if len(out) >= MaxKnowledgeURLs {
			break
		}
	}
	return out
}

func isTrackingURL(raw string) bool {
	lower := strings.ToLower(raw)
	for _, frag := range trackingHostFrags {
		if strings.Contains(lower, frag) {
			return true
		}
	}
	return false
}
