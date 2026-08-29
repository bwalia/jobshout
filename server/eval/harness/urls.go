package harness

import "regexp"

// urlPattern matches bare and markdown-embedded http(s) URLs.
//
// The trailing class deliberately excludes the characters that commonly abut a
// URL in prose — closing brackets, quotes, commas and full stops — so that
// "see https://example.com/x." yields the URL without the sentence's full stop.
var urlPattern = regexp.MustCompile(`https?://[^\s<>"'\)\]]+[^\s<>"'\)\]\.,;:!?]`)

// URLsIn extracts every http(s) URL from text.
//
// Used by the fabrication guard: the URLs a draft cites must be a subset of the
// URLs the research brief actually returned.
func URLsIn(text string) []string {
	found := urlPattern.FindAllString(text, -1)
	if found == nil {
		return nil
	}
	seen := make(map[string]bool, len(found))
	out := make([]string, 0, len(found))
	for _, u := range found {
		k := normaliseURL(u)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, u)
	}
	return out
}
