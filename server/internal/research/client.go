package research

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// Client is the aggregate research surface: one Fetch, one Search across every
// search backend, and one Trending sweep across every listing backend.
//
// Fanning out and merging here rather than in each caller means a new backend
// is added in one place and every consumer — the agent tools, the Research
// Agent, and Phase 2's topic discovery — picks it up without changing.
type Client struct {
	fetcher   Fetcher
	searchers []Searcher
	listers   []Lister
	logger    *zap.Logger
}

// New wires the default backends: GitHub's API and Jina Reader for retrieval,
// Hacker News and arXiv for search, and Hacker News, the blog feeds and arXiv
// for trending.
//
// githubToken is optional — GitHub's API works without one at 60 requests an
// hour, and a token raises that to 5000.
func New(logger *zap.Logger, githubToken string) *Client {
	hn := NewHNClient()
	arxiv := NewArxivClient(nil)
	feeds := NewFeedClient(nil)
	reddit := NewRedditClient(nil)
	youtube := NewYouTubeClient(nil)

	return &Client{
		fetcher: NewRoutingFetcher(NewGitHubFetcher(githubToken), youtube, NewJinaFetcher()),
		// Reddit is listed last on both paths. Its public feeds throttle per IP
		// and contribute nothing when they do, so the backends that always
		// answer get to fill the result set first.
		// YouTube is a lister but not a searcher: there is no free keyless
		// route to YouTube search, so talks are discovered through channel
		// feeds the same way the engineering blogs are.
		searchers: []Searcher{hn, arxiv, reddit},
		listers:   []Lister{hn, feeds, arxiv, reddit, youtube},
		logger:    logger,
	}
}

// RoutingFetcher sends each URL to whichever backend can actually read it.
//
// It exists for GitHub specifically: Jina Reader refuses github.com to
// anonymous callers, so without this a large share of infrastructure sources
// are simply unreadable. Rather than special-casing that inside the Jina
// client, the decision lives here, where adding another specialised backend
// later is one more branch instead of a rewrite.
type RoutingFetcher struct {
	github   *GitHubFetcher
	youtube  *YouTubeClient
	fallback Fetcher
}

// NewRoutingFetcher builds a fetcher that prefers the specialised backends for
// URLs they handle, and the fallback for everything else.
func NewRoutingFetcher(github *GitHubFetcher, youtube *YouTubeClient, fallback Fetcher) *RoutingFetcher {
	return &RoutingFetcher{github: github, youtube: youtube, fallback: fallback}
}

// Fetch routes to the GitHub API for URLs it can serve, and to the fallback
// for everything else.
//
// A GitHub failure is returned rather than retried through the fallback: the
// fallback is Jina, which blocks GitHub anyway, so retrying would only replace
// a precise error ("rate limit exhausted; set GITHUB_TOKEN") with a vague one
// after a slow round trip.
func (r *RoutingFetcher) Fetch(ctx context.Context, rawURL string) (*Document, error) {
	if r.github != nil && r.github.Handles(rawURL) {
		return r.github.Fetch(ctx, rawURL)
	}
	// Jina answers 401 on youtube.com, so a video URL that fell through would
	// simply be unreadable.
	if r.youtube != nil && r.youtube.Handles(rawURL) {
		return r.youtube.Fetch(ctx, rawURL)
	}
	if r.fallback == nil {
		return nil, fmt.Errorf("research: no fetcher can handle %q", rawURL)
	}
	return r.fallback.Fetch(ctx, rawURL)
}

// NewWith builds a Client over explicit backends. Used by tests and by any
// caller that wants a narrower set than the default.
func NewWith(fetcher Fetcher, searchers []Searcher, listers []Lister, logger *zap.Logger) *Client {
	return &Client{fetcher: fetcher, searchers: searchers, listers: listers, logger: logger}
}

// Fetch retrieves a URL and extracts its readable body.
func (c *Client) Fetch(ctx context.Context, rawURL string) (*Document, error) {
	if c.fetcher == nil {
		return nil, fmt.Errorf("research: no fetcher configured")
	}
	return c.fetcher.Fetch(ctx, rawURL)
}

// Search queries every backend concurrently and returns the merged, de-duplicated
// results.
//
// Results are interleaved by backend rather than concatenated: a caller asking
// for ten sources on an AI topic should get a mix of what the industry is
// reading and what the papers say, not ten of whichever backend answered with
// more rows.
func (c *Client) Search(ctx context.Context, query string, limit int) ([]Source, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("research: search query is required")
	}
	if len(c.searchers) == 0 {
		return nil, fmt.Errorf("research: no search backends configured")
	}
	limit = clampLimit(limit)

	// Buckets are indexed by backend position rather than appended on
	// completion. Append order is whichever backend answered first, which would
	// make both the interleaving and which copy of a duplicate survives vary
	// run to run — the same query returning a differently ordered list each
	// time makes a run impossible to reproduce or reason about.
	var (
		mu       sync.Mutex
		buckets  = make([][]Source, len(c.searchers))
		failures []string
		wg       sync.WaitGroup
	)

	for i, s := range c.searchers {
		wg.Add(1)
		go func(i int, s Searcher) {
			defer wg.Done()
			// Each backend is asked for the full limit. After interleaving and
			// de-duplication the merged list is trimmed back, so a backend that
			// returns mostly duplicates does not shrink the final count.
			found, err := s.Search(ctx, query, limit)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", s.Name(), err))
				return
			}
			buckets[i] = found
		}(i, s)
	}
	wg.Wait()

	merged := dedupeSources(interleaveSources(buckets))

	if len(merged) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("research: every search backend failed: %s", strings.Join(failures, "; "))
	}
	if len(failures) > 0 && c.logger != nil {
		c.logger.Warn("research: some search backends failed",
			zap.String("query", query),
			zap.Strings("failures", failures),
		)
	}

	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// Trending sweeps every listing backend and returns candidate topics.
//
// This is the "what is new here" path that a query search cannot answer, and
// it is what Phase 2's topic discovery is built on.
func (c *Client) Trending(ctx context.Context, limit int) ([]TrendingItem, error) {
	if len(c.listers) == 0 {
		return nil, fmt.Errorf("research: no trending backends configured")
	}
	limit = clampLimit(limit)

	// Indexed by backend position for the same reason as Search — see there.
	var (
		mu       sync.Mutex
		buckets  = make([][]TrendingItem, len(c.listers))
		failures []string
		wg       sync.WaitGroup
	)

	for i, l := range c.listers {
		wg.Add(1)
		go func(i int, l Lister) {
			defer wg.Done()
			found, err := l.List(ctx, limit)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", l.Name(), err))
				return
			}
			buckets[i] = found
		}(i, l)
	}
	wg.Wait()

	merged := dedupeTrending(interleaveTrending(buckets))

	if len(merged) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("research: every trending backend failed: %s", strings.Join(failures, "; "))
	}
	if len(failures) > 0 && c.logger != nil {
		c.logger.Warn("research: some trending backends failed", zap.Strings("failures", failures))
	}

	if len(merged) > limit {
		merged = merged[:limit]
	}
	return merged, nil
}

// interleaveSources takes one result set per backend and returns them round
// robin: first result from each backend, then the second from each, and so on.
func interleaveSources(buckets [][]Source) []Source {
	total := 0
	for _, b := range buckets {
		total += len(b)
	}
	out := make([]Source, 0, total)

	for i := 0; ; i++ {
		progressed := false
		for _, b := range buckets {
			if i < len(b) {
				out = append(out, b[i])
				progressed = true
			}
		}
		if !progressed {
			return out
		}
	}
}

// interleaveTrending is interleaveSources for TrendingItem, keeping each
// channel represented near the top of the list.
func interleaveTrending(buckets [][]TrendingItem) []TrendingItem {
	total := 0
	for _, b := range buckets {
		total += len(b)
	}
	out := make([]TrendingItem, 0, total)

	for i := 0; ; i++ {
		progressed := false
		for _, b := range buckets {
			if i < len(b) {
				out = append(out, b[i])
				progressed = true
			}
		}
		if !progressed {
			return out
		}
	}
}

// dedupeTrending removes repeated URLs across channels, keeping the first
// occurrence in position but carrying over the best popularity signal seen.
//
// The score merge matters because the same article routinely appears in both a
// ranked channel and an unranked one — trending on Hacker News with 400 points
// and also sitting in the publisher's own RSS feed, where Score is 0. Whichever
// copy happens to come first, the item is equally popular, so discarding the
// score because RSS was interleaved earlier would hand Phase 2's ranking a
// zero for one of the most-read articles of the day.
func dedupeTrending(in []TrendingItem) []TrendingItem {
	pos := make(map[string]int, len(in))
	out := make([]TrendingItem, 0, len(in))
	for _, it := range in {
		key := canonicalURL(it.URL)
		if key == "" {
			continue
		}
		if i, dup := pos[key]; dup {
			if it.Score > out[i].Score {
				out[i].Score = it.Score
				out[i].Channel = it.Channel
			}
			continue
		}
		pos[key] = len(out)
		out = append(out, it)
	}
	return out
}
