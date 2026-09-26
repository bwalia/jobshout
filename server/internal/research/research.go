// Package research gives agents grounded access to what is currently published
// on the internet: search for sources on a topic, list what is new in a domain,
// and retrieve a page as clean text.
//
// Every backend here is free and keyless — Jina Reader for extraction, the
// Hacker News Algolia index for tech search and trending, arXiv for papers, and
// plain RSS. That is a deliberate constraint rather than a temporary one: the
// article pipeline runs unattended on a schedule, so a research layer that
// depends on a paid key or an expiring session cookie is a layer that silently
// stops working at 3am. Anything needing credentials belongs behind a separate,
// optional provider — not in this package's default path.
//
// The package holds no LLM. It returns sources and documents; deciding what to
// search for and what a document means is the caller's job. That split is what
// lets the same clients back both the agent-facing tools in internal/tools and
// the Research Agent's own loop without a dependency cycle.
package research

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Source is a citable document the agent has found but not necessarily read.
// It carries enough to rank and de-duplicate candidates before paying the cost
// of fetching each one.
type Source struct {
	URL   string `json:"url"`
	Title string `json:"title"`
	// Site is the URL's host, lowercased and stripped of "www.". It is what a
	// reader recognises ("kubernetes.io") and what source-diversity checks
	// group on, so it is stored rather than re-derived at each use.
	Site string `json:"site"`
	// PublishedAt is nil when the backend does not report one. Recency matters
	// for a "what's current" article, so an unknown date is represented
	// honestly rather than defaulted to now.
	PublishedAt *time.Time `json:"published_at,omitempty"`
	// Excerpt is a short snippet from the backend, used to judge relevance
	// before fetching. It is not citable on its own — it is usually truncated
	// mid-sentence and sometimes just the first paragraph of boilerplate.
	Excerpt string `json:"excerpt,omitempty"`
}

// Document is a Source that has actually been retrieved and extracted.
//
// A claim may only be cited against a Document, never a Source: the whole point
// of the citation pass is that something read the page. Keeping them as
// separate types means "I found this URL" cannot be mistaken for "I read this
// page" anywhere downstream.
type Document struct {
	Source
	// Text is the extracted body as markdown.
	Text string `json:"text"`
	// FetchedAt records when the retrieval happened, so a cached document can
	// be aged out and a citation can state when the page said what it said.
	FetchedAt time.Time `json:"fetched_at"`
}

// TrendingItem is a candidate topic surfaced from a domain feed rather than
// found by a query.
type TrendingItem struct {
	Source
	// Score is the backend's own popularity signal — HN points, and 0 for feeds
	// that publish no ranking. Only comparable within a Channel.
	Score int `json:"score"`
	// Channel identifies which backend produced this ("hackernews",
	// "rss:kubernetes", "arxiv:cs.AI"), so a ranker can enforce spread across
	// sources instead of returning ten items from whichever feed was busiest.
	Channel string `json:"channel"`
}

// Searcher finds sources matching a query.
type Searcher interface {
	Search(ctx context.Context, query string, limit int) ([]Source, error)
	// Name identifies the backend in logs and in Channel values.
	Name() string
}

// Fetcher retrieves a URL and extracts its readable body.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string) (*Document, error)
}

// Lister returns what is recent in a domain, with no query.
//
// This is deliberately not expressible as Search(""): trending discovery has no
// query to search for, and a backend answers "what is new here" through a
// different endpoint than "what matches this string". Keeping it a separate
// interface is what lets topic discovery reuse these same clients instead of
// needing its own integration.
type Lister interface {
	List(ctx context.Context, limit int) ([]TrendingItem, error)
	Name() string
}

// DefaultLimit is the number of results returned when a caller does not say.
const DefaultLimit = 10

// maxLimit caps any single backend call. The consumer of these results is an
// LLM context window, and a hundred search hits crowd out the actual article.
const maxLimit = 50

// clampLimit normalises a caller-supplied limit.
func clampLimit(n int) int {
	if n <= 0 {
		return DefaultLimit
	}
	if n > maxLimit {
		return maxLimit
	}
	return n
}

// siteOf extracts the display host from a URL: lowercased, "www." stripped.
// Returns "" for anything unparseable, which callers treat as an unknown site
// rather than an error — a source with an odd URL is still a source.
func siteOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// validateURL rejects anything that is not a plain http(s) URL.
//
// Note there is no private-address check here, and none is needed: every fetch
// in this package is performed by Jina Reader, not by this process, so a URL
// pointing at 169.254.169.254 or a cluster-internal service resolves in Jina's
// network and not ours. That property is load-bearing — a direct-fetch fallback
// added later would reintroduce SSRF and would need its own guard.
func validateURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", fmt.Errorf("research: url is required")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("research: parse url %q: %w", rawURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("research: url %q must be http or https", rawURL)
	}
	if u.Host == "" {
		return "", fmt.Errorf("research: url %q has no host", rawURL)
	}
	return u.String(), nil
}

// truncate shortens s to at most n runes, appending an ellipsis when it cut.
// Used on excerpts and document bodies before they reach a prompt.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}

// dedupeSources removes repeated URLs, keeping first-seen order.
//
// Backends overlap heavily — an article that trends on HN is usually also in
// the site's own RSS feed — and a duplicate source read twice is both a wasted
// fetch and a citation list that looks padded.
func dedupeSources(in []Source) []Source {
	seen := make(map[string]struct{}, len(in))
	out := make([]Source, 0, len(in))
	for _, s := range in {
		key := canonicalURL(s.URL)
		if key == "" {
			continue
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

// canonicalURL normalises a URL for equality checks: scheme and host
// lowercased, a trailing slash removed, and tracking query parameters dropped.
// It is only ever used as a map key — the original URL is what gets cited.
func canonicalURL(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	u.Path = strings.TrimSuffix(u.Path, "/")
	u.Fragment = ""

	// utm_* and friends make the same page look like several distinct sources.
	if q := u.Query(); len(q) > 0 {
		for key := range q {
			if strings.HasPrefix(key, "utm_") || key == "ref" || key == "source" {
				q.Del(key)
			}
		}
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// withoutSeeds drops search results that are seeds already read, compared by
// canonical URL so a tracking parameter or trailing slash does not let the
// same page be read — and cited — twice.
func withoutSeeds(searched []Source, seeds []Document) []Source {
	if len(seeds) == 0 {
		return searched
	}
	read := make(map[string]struct{}, len(seeds))
	for _, d := range seeds {
		read[canonicalURL(d.URL)] = struct{}{}
	}
	out := make([]Source, 0, len(searched))
	for _, s := range searched {
		if _, ok := read[canonicalURL(s.URL)]; ok {
			continue
		}
		out = append(out, s)
	}
	return out
}

// cleanFocus trims the caller's focus areas and drops empty ones.
func cleanFocus(focus []string) []string {
	out := make([]string, 0, len(focus))
	for _, f := range focus {
		if s := strings.TrimSpace(f); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// focusPlanBlock is the planner prompt's focus section. Empty when there are
// no focus areas, so an unfocused request sees exactly the prompt it always did.
func focusPlanBlock(focus []string) string {
	focus = cleanFocus(focus)
	if len(focus) == 0 {
		return ""
	}
	return "\n\nFOCUS AREAS (the subject area this article must stay within):\n- " +
		strings.Join(focus, "\n- ") +
		"\n\nEvery query must stay inside the topic's subject area as defined by these focus\n" +
		"areas. A query that would find material sharing the topic's words but not its\n" +
		"subject — a different field, product or problem — is a wasted search. Leave it out."
}

// focusSelectBlock is the source-selection prompt's focus section, empty when
// there are no focus areas for the same reason as focusPlanBlock.
func focusSelectBlock(focus []string) string {
	focus = cleanFocus(focus)
	if len(focus) == 0 {
		return ""
	}
	return "\n\nFOCUS AREAS (the article must stay within these):\n- " +
		strings.Join(focus, "\n- ") +
		"\n\nJudge each source against the topic AS PART OF these focus areas. A source\n" +
		"that is not about them is off-subject however many words it shares with the\n" +
		"topic — reject it."
}

// focusWords reduces a phrase to its meaningful lowercase words, in order.
// Unlike significantWords it keeps two-letter words, because "AI" is often the
// whole point of a focus area.
func focusWords(s string) []string {
	var out []string
	for _, w := range strings.Fields(normaliseForCompare(s)) {
		if len(w) < 2 {
			continue
		}
		if _, skip := stopWords[w]; skip {
			continue
		}
		out = append(out, w)
	}
	return out
}

// focusQueryTerms caps how much of a focus area goes into the added query. The
// planner is told to write two-to-four-word queries because long ones match
// nothing, and the added query should follow the same rule.
const focusQueryTerms = 3

// ensureFocusQuery guarantees that at least one query touches the focus areas.
//
// The planner is told to stay inside them, but a model can still turn a topic
// into queries about a neighbouring field — "Efficiently Observing AI Agents
// with Prompt Caching" became efficient-inference searches that never said
// "observability". So the check is deterministic and deliberately crude: if no
// query shares a word with any focus area, one query is added, built from the
// topic's key noun (its last meaningful word, which in an English noun phrase
// is usually the head) and the focus area that shares the most words with the
// topic (the first one on a tie). With no focus areas, queries pass unchanged.
func ensureFocusQuery(topic string, queries []string, focus []string) []string {
	focus = cleanFocus(focus)
	if len(focus) == 0 {
		return queries
	}

	terms := make(map[string]struct{})
	for _, area := range focus {
		for _, w := range focusWords(area) {
			terms[w] = struct{}{}
		}
	}
	if len(terms) == 0 {
		return queries
	}
	for _, q := range queries {
		for _, w := range focusWords(q) {
			if _, ok := terms[w]; ok {
				return queries
			}
		}
	}

	topicWords := focusWords(topic)
	inTopic := make(map[string]struct{}, len(topicWords))
	for _, w := range topicWords {
		inTopic[w] = struct{}{}
	}
	var closest []string
	best := -1
	for _, area := range focus {
		words := focusWords(area)
		if len(words) == 0 {
			continue
		}
		shared := 0
		for _, w := range words {
			if _, ok := inTopic[w]; ok {
				shared++
			}
		}
		if shared > best {
			best, closest = shared, words
		}
	}

	parts := make([]string, 0, focusQueryTerms+1)
	if len(topicWords) > 0 {
		parts = append(parts, topicWords[len(topicWords)-1])
	}
	for _, w := range closest[:min(len(closest), focusQueryTerms)] {
		if len(parts) > 0 && w == parts[0] {
			continue
		}
		parts = append(parts, w)
	}
	return append(append([]string{}, queries...), strings.Join(parts, " "))
}
