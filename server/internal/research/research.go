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
