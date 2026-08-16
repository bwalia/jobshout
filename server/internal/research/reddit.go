package research

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// RedditBase is the public site. Reddit publishes Atom for both search and
// subreddit listings, which is the only route into it that needs no account —
// the JSON API answers 403 to anything unauthenticated, and so does Jina
// Reader, which gets the same "log in or use your developer token" page.
const RedditBase = "https://www.reddit.com"

// DefaultSubreddits are the communities the trending sweep reads.
//
// Reddit earns its place next to the engineering blogs because it carries
// something they structurally cannot: practitioners saying what broke. A vendor
// post explains how a thing is meant to work; a thread explains what happened
// when someone ran it on a Tuesday. For an article about operating software,
// that is often the more useful half.
var DefaultSubreddits = []string{
	"kubernetes", "devops", "programming", "golang",
	"MachineLearning", "LocalLLaMA", "sre", "dataengineering",
}

// redditRateLimited is the reply Reddit gives an IP that has asked too often.
// It is called out separately because it is not a failure of the query — the
// same request will work again shortly — and because the fix is a credential
// rather than a different search.
const redditRateLimited = 429

// RedditClient reads Reddit through its public Atom feeds.
//
// Search is rate limited hard and per-IP: a handful of requests in quick
// succession is enough to earn a 429 for a while. That is survivable because
// nothing here is load-bearing — Reddit is one searcher among several, and a
// throttled one contributes nothing rather than failing the research.
type RedditClient struct {
	baseURL    string
	subreddits []string
	client     *http.Client
}

// NewRedditClient builds a client over the given subreddits, or
// DefaultSubreddits when the list is empty.
func NewRedditClient(subreddits []string) *RedditClient {
	if len(subreddits) == 0 {
		subreddits = DefaultSubreddits
	}
	return &RedditClient{
		baseURL:    RedditBase,
		subreddits: subreddits,
		client:     &http.Client{Timeout: 20 * time.Second},
	}
}

// Name identifies this backend.
func (c *RedditClient) Name() string { return "reddit" }

// Search returns threads matching a query.
func (c *RedditClient) Search(ctx context.Context, query string, limit int) ([]Source, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("research: reddit: query is required")
	}
	limit = clampLimit(limit)

	q := url.Values{}
	q.Set("q", query)
	q.Set("sort", "relevance")
	// A year, because the useful thread about a piece of infrastructure is
	// often not from this week — it is from whenever people were migrating.
	q.Set("t", "year")

	body, err := c.get(ctx, "/search.rss?"+q.Encode())
	if err != nil {
		return nil, err
	}

	items, err := c.parse(body, "")
	if err != nil {
		return nil, err
	}

	out := make([]Source, 0, len(items))
	for _, it := range items {
		out = append(out, it.Source)
		if len(out) >= limit {
			break
		}
	}
	return dedupeSources(out), nil
}

// List returns what the configured subreddits are upvoting this week.
func (c *RedditClient) List(ctx context.Context, limit int) ([]TrendingItem, error) {
	limit = clampLimit(limit)

	// Reddit serves a combined feed for several subreddits in one request,
	// which matters when the alternative is one throttled request per
	// community.
	path := "/r/" + strings.Join(c.subreddits, "+") + "/top/.rss?t=week"

	body, err := c.get(ctx, path)
	if err != nil {
		return nil, err
	}

	items, err := c.parse(body, c.Name())
	if err != nil {
		return nil, err
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return items, nil
}

// get performs one request against the public site.
func (c *RedditClient) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, fmt.Errorf("research: reddit: build request: %w", err)
	}
	// Reddit refuses a bare Go user agent outright, and asks that callers
	// identify themselves.
	req.Header.Set("User-Agent", "JobShout-ResearchAgent/1.0 (+https://github.com/jobshout)")
	req.Header.Set("Accept", "application/atom+xml, application/xml, text/xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("research: reddit: %w", err)
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == redditRateLimited:
		return nil, fmt.Errorf(
			"research: reddit: rate limited — the public feeds throttle per IP; " +
				"this query contributed nothing to the research")
	case resp.StatusCode == http.StatusForbidden:
		return nil, fmt.Errorf("research: reddit: blocked (HTTP 403) — the public feed refused this request")
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return nil, fmt.Errorf("research: reddit: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBody))
	if err != nil {
		return nil, fmt.Errorf("research: reddit: read body: %w", err)
	}
	return body, nil
}

// parse turns a Reddit Atom feed into items.
//
// It reuses the feed decoder rather than parseFeed itself: parseFeed stamps an
// "rss:" channel and applies the trending age cutoff, and a thread from eight
// months ago is exactly what a search is meant to surface.
func (c *RedditClient) parse(body []byte, channel string) ([]TrendingItem, error) {
	doc, err := decodeFeed(body)
	if err != nil {
		return nil, fmt.Errorf("research: reddit: %w", err)
	}

	entries := doc.Entries
	if len(entries) == 0 {
		entries = doc.Channel.Items
	}

	out := make([]TrendingItem, 0, len(entries))
	for _, e := range entries {
		src := e.toSource()
		if src.URL == "" || !isRedditThread(src.URL) {
			continue
		}
		// The feed's own site is reddit.com whichever subreddit it came from,
		// which is what a reader recognises and what source-diversity checks
		// should group on.
		src.Site = "reddit.com"
		out = append(out, TrendingItem{
			Source: src,
			// Reddit does not put scores in its feeds. Leaving this 0 keeps
			// Score honest — it is only comparable within a channel anyway.
			Score:   0,
			Channel: channel,
		})
	}
	return out, nil
}

// isRedditThread filters out the feed's self-links and non-thread URLs, so a
// citation always points at a discussion rather than at a listing page.
func isRedditThread(rawURL string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	if !strings.HasSuffix(strings.ToLower(u.Host), "reddit.com") {
		return false
	}
	return strings.Contains(u.Path, "/comments/")
}
