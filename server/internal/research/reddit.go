package research

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
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

// RedlibMirrors are public instances of Redlib, the open-source Reddit
// front-end (redlib-org/redlib, AGPL).
//
// They exist here for one reason: reddit.com's own search feed throttles per IP
// after a handful of requests, and three consecutive queries were enough to
// earn a 429 during testing. The same three queries through a Redlib mirror all
// answered 200. Redlib parses the JSON endpoints Reddit's own site uses, so the
// results are Reddit's, just without the rate limit aimed at us.
//
// Mirrors are community-run and individually unreliable — of six listed, two
// answered 403 and one served nothing — so they are tried in order until one
// responds, and the whole thing falls back to reddit.com if none do.
var RedlibMirrors = []string{
	"https://safereddit.com",
	"https://red.artemislena.eu",
	"https://redlib.privacyredirect.com",
}

// RedditClient reads Reddit through Redlib mirrors, falling back to Reddit's
// own public Atom feeds.
//
// Nothing here is load-bearing: Reddit is one searcher among several, and a
// throttled or unreachable one contributes nothing rather than failing the
// research.
type RedditClient struct {
	baseURL    string
	mirrors    []string
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
		mirrors:    RedlibMirrors,
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

	// Mirrors first: reddit.com's own search throttles us within a few
	// requests, and an article's research makes several.
	if sources := c.searchMirrors(ctx, q, limit); len(sources) > 0 {
		return sources, nil
	}

	body, err := c.get(ctx, c.baseURL, "/search.rss?"+q.Encode())
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

// searchMirrors tries each Redlib mirror until one answers, returning nil when
// none do so the caller can fall back.
func (c *RedditClient) searchMirrors(ctx context.Context, q url.Values, limit int) []Source {
	for _, mirror := range c.mirrors {
		body, err := c.get(ctx, mirror, "/search?"+q.Encode())
		if err != nil {
			continue
		}
		sources := parseRedlibThreads(string(body), limit)
		if len(sources) > 0 {
			return sources
		}
	}
	return nil
}

// List returns what the configured subreddits are upvoting this week.
func (c *RedditClient) List(ctx context.Context, limit int) ([]TrendingItem, error) {
	limit = clampLimit(limit)

	// Reddit serves a combined feed for several subreddits in one request,
	// which matters when the alternative is one throttled request per
	// community.
	path := "/r/" + strings.Join(c.subreddits, "+") + "/top/.rss?t=week"

	body, err := c.get(ctx, c.baseURL, path)
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
func (c *RedditClient) get(ctx context.Context, base, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+path, nil)
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

// redlibThreadPattern matches a Redlib search result: a link to a thread,
// followed by the post title in the markup.
//
// Parsing HTML with a regex is normally a mistake, but the target here is one
// narrow, stable shape — an anchor whose href contains "/comments/" — rather
// than a document structure. A mirror changing its markup makes this return
// nothing, which falls back to reddit.com rather than returning wrong results.
var redlibThreadPattern = regexp.MustCompile(
	`href="(/r/[^"]*?/comments/[^"]+)"[^>]*>(?:\s*<[^>]+>)*\s*([^<]{10,200})`)

// redlibNoise matches the link text Redlib puts on things that are not titles.
var redlibNoise = regexp.MustCompile(`^\s*(\d+\s+comments?|my comment|\d+\s*(points?|pts))\s*$`)

// parseRedlibThreads extracts Reddit threads from a Redlib search page.
//
// The URLs are rewritten back to reddit.com. A mirror is a way of reading
// Reddit, not a place to send a reader: mirrors come and go — two of the six
// listed were already refusing requests when this was written — and a citation
// pointing at one would rot with it. The thread on reddit.com will outlive any
// front-end for it.
func parseRedlibThreads(page string, limit int) []Source {
	matches := redlibThreadPattern.FindAllStringSubmatch(page, -1)

	out := make([]Source, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))

	for _, m := range matches {
		path, title := m[1], strings.TrimSpace(html.UnescapeString(m[2]))
		if title == "" || redlibNoise.MatchString(title) {
			continue
		}
		// Strip Redlib's query parameters; the canonical thread has none.
		if i := strings.IndexAny(path, "?#"); i >= 0 {
			path = path[:i]
		}
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}

		out = append(out, Source{
			URL:   RedditBase + path,
			Title: title,
			Site:  "reddit.com",
		})
		if len(out) >= limit {
			break
		}
	}
	return out
}
