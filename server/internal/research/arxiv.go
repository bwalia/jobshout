package research

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// ArxivBaseURL is the public arXiv API. Keyless, but it asks callers to keep
// request rates modest, which the agent's usage pattern (a handful of queries
// per article) sits comfortably inside.
const ArxivBaseURL = "https://export.arxiv.org/api/query"

// ArxivCategories are the listings trending discovery sweeps: AI, machine
// learning, computation and language, and distributed/cluster computing. They
// are the arXiv sections where the papers behind an infrastructure or AI story
// actually land.
var ArxivCategories = []string{"cs.AI", "cs.LG", "cs.CL", "cs.DC"}

// ArxivClient searches and lists arXiv papers.
//
// It earns its place next to the blog feeds because it is the primary source
// for AI claims: an article asserting a technique works should be able to cite
// the paper, not a vendor's summary of the paper.
type ArxivClient struct {
	baseURL    string
	categories []string
	client     *http.Client
}

// NewArxivClient builds a client over the given categories, or
// ArxivCategories when the list is empty.
func NewArxivClient(categories []string) *ArxivClient {
	if len(categories) == 0 {
		categories = ArxivCategories
	}
	return &ArxivClient{
		baseURL:    ArxivBaseURL,
		categories: categories,
		client:     &http.Client{Timeout: 25 * time.Second},
	}
}

// Name identifies this backend.
func (c *ArxivClient) Name() string { return "arxiv" }

// Search returns papers matching query, most recently submitted first.
//
// Recency is chosen over arXiv's relevance ordering deliberately. The agent
// searches arXiv when it wants to know the current state of a technique, and
// relevance ranking surfaces the seminal 2017 paper for almost any query —
// accurate, but not what "what is happening now" needs.
func (c *ArxivClient) Search(ctx context.Context, query string, limit int) ([]Source, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("research: arxiv: query is required")
	}

	// Quoting makes a multi-word query a phrase match. Unquoted, arXiv ORs the
	// terms and a two-word query returns most of the archive.
	items, err := c.query(ctx, fmt.Sprintf("all:%q", query), limit)
	if err != nil {
		return nil, err
	}

	out := make([]Source, 0, len(items))
	for _, it := range items {
		if src := it.toSource(); src.URL != "" {
			out = append(out, src)
		}
	}
	return dedupeSources(out), nil
}

// List returns the newest papers across the configured categories.
func (c *ArxivClient) List(ctx context.Context, limit int) ([]TrendingItem, error) {
	limit = clampLimit(limit)

	// One OR'd query rather than one request per category: arXiv asks callers
	// to be gentle, and this is the same answer in a single round trip.
	terms := make([]string, 0, len(c.categories))
	for _, cat := range c.categories {
		terms = append(terms, "cat:"+cat)
	}

	items, err := c.query(ctx, strings.Join(terms, " OR "), limit)
	if err != nil {
		return nil, err
	}

	out := make([]TrendingItem, 0, len(items))
	for _, it := range items {
		src := it.toSource()
		if src.URL == "" {
			continue
		}
		out = append(out, TrendingItem{
			Source: src,
			// arXiv publishes no citation or attention count in this API, so
			// there is no honest popularity signal to report.
			Score:   0,
			Channel: c.Name(),
		})
	}
	return out, nil
}

// query performs one API call and returns the raw Atom entries.
func (c *ArxivClient) query(ctx context.Context, searchQuery string, limit int) ([]xmlItem, error) {
	limit = clampLimit(limit)

	q := url.Values{}
	q.Set("search_query", searchQuery)
	q.Set("max_results", strconv.Itoa(limit))
	q.Set("sortBy", "submittedDate")
	q.Set("sortOrder", "descending")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("research: arxiv: build request: %w", err)
	}
	req.Header.Set("User-Agent", "JobShout-ResearchAgent/1.0 (+https://github.com/jobshout)")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("research: arxiv: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("research: arxiv: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBody))
	if err != nil {
		return nil, fmt.Errorf("research: arxiv: read body: %w", err)
	}

	// arXiv speaks Atom, so the feed parser's entry handling applies directly.
	// parseFeed is not reused wholesale because it stamps an "rss:" channel and
	// applies the trending age cutoff, neither of which suits a paper search.
	doc, err := decodeFeed(body)
	if err != nil {
		return nil, fmt.Errorf("research: arxiv: %w", err)
	}
	return doc.Entries, nil
}
