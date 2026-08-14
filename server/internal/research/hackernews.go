package research

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// HNBaseURL is the Hacker News Algolia index. It is public, keyless and
// unmetered, and it indexes the URL of every story submitted — which makes it
// both a search engine over what the industry is actually reading and, via the
// front_page tag, a live trending feed.
//
// Its bias is worth naming: this is what Hacker News surfaced, not what the web
// contains. For articles about tech, AI and infrastructure that bias points the
// right way. It would be the wrong tool for a general-purpose researcher.
const HNBaseURL = "https://hn.algolia.com/api/v1"

// hnMinPoints filters search results to stories the community engaged with at
// all. Anything submitted to HN is in this index, including link spam that
// nobody upvoted; a low floor removes most of it while keeping niche but real
// technical posts, which often peak in the teens.
const hnMinPoints = 10

// hnItemURL is where a story with no external link lives (Ask HN, and text
// posts generally).
const hnItemURL = "https://news.ycombinator.com/item?id="

// HNClient searches and lists Hacker News stories. It satisfies both Searcher
// and Lister — the same index answers "what has been written about X" and
// "what is on the front page right now" through different endpoints.
type HNClient struct {
	baseURL string
	client  *http.Client
}

// NewHNClient builds a client against the public Algolia index.
func NewHNClient() *HNClient {
	return &HNClient{
		baseURL: HNBaseURL,
		client:  &http.Client{Timeout: 20 * time.Second},
	}
}

// Name identifies this backend.
func (c *HNClient) Name() string { return "hackernews" }

// hnResponse is the subset of the Algolia envelope we read.
type hnResponse struct {
	Hits []struct {
		ObjectID    string   `json:"objectID"`
		Title       string   `json:"title"`
		URL         string   `json:"url"`
		Points      int      `json:"points"`
		NumComments int      `json:"num_comments"`
		CreatedAt   string   `json:"created_at"`
		StoryText   string   `json:"story_text"`
		Tags        []string `json:"_tags"`
	} `json:"hits"`
}

// Search returns stories matching query, most relevant first.
func (c *HNClient) Search(ctx context.Context, query string, limit int) ([]Source, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("research: hackernews: query is required")
	}
	limit = clampLimit(limit)

	q := url.Values{}
	q.Set("query", query)
	q.Set("tags", "story")
	q.Set("hitsPerPage", strconv.Itoa(limit))
	q.Set("numericFilters", fmt.Sprintf("points>=%d", hnMinPoints))
	// Without this the index requires every term to match, and a query like
	// "kubernetes gateway api ingress replacement production" returns nothing
	// at all — which is exactly the shape of query a language model writes when
	// asked what to search for. Algolia tries the strict conjunction first and
	// only falls back to ranking by how many terms matched when that finds
	// nothing, so precision is preserved wherever it actually exists.
	q.Set("removeWordsIfNoResults", "allOptional")

	hits, err := c.get(ctx, "/search", q)
	if err != nil {
		return nil, err
	}

	out := make([]Source, 0, len(hits.Hits))
	for _, h := range hits.Hits {
		src := hnSource(h.Title, h.URL, h.ObjectID, h.StoryText, h.CreatedAt)
		if src.URL == "" {
			continue
		}
		out = append(out, src)
	}
	return dedupeSources(out), nil
}

// List returns the current front page — the closest thing to a canonical
// "what is the industry talking about today" feed.
//
// It uses the plain search endpoint with the front_page tag rather than
// search_by_date: recency alone surfaces every new submission including the
// ones nobody read, whereas the front page is already filtered by attention.
func (c *HNClient) List(ctx context.Context, limit int) ([]TrendingItem, error) {
	limit = clampLimit(limit)

	q := url.Values{}
	q.Set("tags", "front_page")
	q.Set("hitsPerPage", strconv.Itoa(limit))

	hits, err := c.get(ctx, "/search", q)
	if err != nil {
		return nil, err
	}

	out := make([]TrendingItem, 0, len(hits.Hits))
	for _, h := range hits.Hits {
		src := hnSource(h.Title, h.URL, h.ObjectID, h.StoryText, h.CreatedAt)
		if src.URL == "" {
			continue
		}
		out = append(out, TrendingItem{
			Source:  src,
			Score:   h.Points,
			Channel: c.Name(),
		})
	}
	return out, nil
}

// get performs a request against the index and decodes the envelope.
func (c *HNClient) get(ctx context.Context, path string, q url.Values) (*hnResponse, error) {
	endpoint := c.baseURL + path + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("research: hackernews: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("research: hackernews: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("research: hackernews: HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("research: hackernews: read body: %w", err)
	}

	var parsed hnResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("research: hackernews: decode: %w", err)
	}
	return &parsed, nil
}

// hnSource builds a Source from one hit.
//
// A story with no url is a text post, which is still worth citing — discussion
// threads are often where the real detail about a release lives — so it falls
// back to its HN item page rather than being dropped.
func hnSource(title, storyURL, objectID, storyText, createdAt string) Source {
	link := strings.TrimSpace(storyURL)
	if link == "" && objectID != "" {
		link = hnItemURL + objectID
	}
	if link == "" {
		return Source{}
	}

	var published *time.Time
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		published = &t
	}

	return Source{
		URL:         link,
		Title:       strings.TrimSpace(title),
		Site:        siteOf(link),
		PublishedAt: published,
		Excerpt:     truncate(strings.TrimSpace(storyText), 300),
	}
}
