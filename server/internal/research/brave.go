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
	"sync"
	"time"
)

// BraveBaseURL is the Brave Search API.
const BraveBaseURL = "https://api.search.brave.com/res/v1"

// braveMaxCount is the most results one request may ask for.
const braveMaxCount = 20

// braveMinInterval spaces requests to the API. The entry plan allows one
// request a second, and research asks several questions back to back: a run's
// four queries, or discovery's search for each focus area. Unspaced, all but
// the first came back 429. A second between searches costs a run a few
// seconds; losing the searches costs it its sources.
const braveMinInterval = 1100 * time.Millisecond

// BraveClient searches the general web.
//
// Hacker News and arXiv only know what engineers and researchers posted. A
// brief about recruitment, regulation or any other non-technical subject found
// nothing relevant there, and the writer was left stretching a speech-to-text
// repository into an article about hiring. This is the backend for everything
// those two do not index.
type BraveClient struct {
	baseURL string
	apiKey  string
	client  *http.Client

	// mu and last space requests braveMinInterval apart. interval is a field
	// so tests need not wait.
	mu       sync.Mutex
	last     time.Time
	interval time.Duration
}

// NewBraveClient builds a client for apiKey.
func NewBraveClient(apiKey string) *BraveClient {
	return &BraveClient{
		baseURL:  BraveBaseURL,
		apiKey:   strings.TrimSpace(apiKey),
		client:   &http.Client{Timeout: 20 * time.Second},
		interval: braveMinInterval,
	}
}

// Name identifies this backend.
func (c *BraveClient) Name() string { return "web" }

// braveResponse is the subset of the Web Search envelope we read.
type braveResponse struct {
	Web struct {
		Results []struct {
			Title       string `json:"title"`
			URL         string `json:"url"`
			Description string `json:"description"`
			// PageAge is when the page was published, when Brave knows.
			PageAge string `json:"page_age"`
		} `json:"results"`
	} `json:"web"`
}

// Search returns web pages matching query, most relevant first.
func (c *BraveClient) Search(ctx context.Context, query string, limit int) ([]Source, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("research: web: query is required")
	}
	limit = min(clampLimit(limit), braveMaxCount)

	q := url.Values{}
	q.Set("q", query)
	q.Set("count", strconv.Itoa(limit))
	// Snippets arrive with <strong> around the matched terms unless asked not to.
	q.Set("text_decorations", "false")
	// Only web pages: the other result types (videos, discussions, FAQ boxes)
	// are not sources an article can be written from.
	q.Set("result_filter", "web")

	parsed, err := c.get(ctx, "/web/search", q)
	if err != nil {
		return nil, err
	}

	out := make([]Source, 0, len(parsed.Web.Results))
	for _, r := range parsed.Web.Results {
		link := strings.TrimSpace(r.URL)
		if link == "" {
			continue
		}
		out = append(out, Source{
			URL:         link,
			Title:       strings.TrimSpace(stripTags(r.Title)),
			Site:        siteOf(link),
			PublishedAt: parseBraveTime(r.PageAge),
			Excerpt:     truncate(strings.TrimSpace(stripTags(r.Description)), 300),
		})
	}
	return dedupeSources(out), nil
}

// get makes one spaced request, and asks once more when told to slow down.
func (c *BraveClient) get(ctx context.Context, path string, q url.Values) (*braveResponse, error) {
	endpoint := c.baseURL + path + "?" + q.Encode()

	for attempt := 0; ; attempt++ {
		if err := c.pace(ctx); err != nil {
			return nil, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("research: web: build request: %w", err)
		}
		req.Header.Set("Accept", "application/json")
		req.Header.Set("X-Subscription-Token", c.apiKey)

		resp, err := c.client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("research: web: %w", err)
		}
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusTooManyRequests && attempt == 0:
			if err := sleepCtx(ctx, c.retryAfter(resp.Header.Get("Retry-After"))); err != nil {
				return nil, err
			}
			continue
		case resp.StatusCode < 200 || resp.StatusCode >= 300:
			return nil, braveError(resp.StatusCode, body)
		}
		if rerr != nil {
			return nil, fmt.Errorf("research: web: read body: %w", rerr)
		}

		var parsed braveResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return nil, fmt.Errorf("research: web: decode: %w", err)
		}
		return &parsed, nil
	}
}

// braveError turns a refusal into something a person can act on.
//
// A rejected key is named, because it is the one failure someone can fix — and
// the API reports it as 422 SUBSCRIPTION_TOKEN_INVALID, not 401, so the status
// alone does not say so. The rest of research carries on with the other
// backends either way.
func braveError(status int, body []byte) error {
	var parsed struct {
		Error struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &parsed)

	code := parsed.Error.Code
	switch {
	case strings.Contains(code, "SUBSCRIPTION_TOKEN"),
		status == http.StatusUnauthorized, status == http.StatusForbidden:
		return fmt.Errorf("research: web: HTTP %d — the key was rejected, check BRAVE_SEARCH_API_KEY", status)
	case code != "":
		return fmt.Errorf("research: web: HTTP %d %s: %s", status, code, truncate(parsed.Error.Detail, 200))
	}
	return fmt.Errorf("research: web: HTTP %d", status)
}

// pace waits until braveMinInterval has passed since the last request. Calls
// queue on the mutex, so concurrent searches go out one at a time.
func (c *BraveClient) pace(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if wait := c.interval - time.Since(c.last); wait > 0 && !c.last.IsZero() {
		if err := sleepCtx(ctx, wait); err != nil {
			return err
		}
	}
	c.last = time.Now()
	return nil
}

// retryAfter reads a Retry-After header in seconds, bounded so a bad value
// cannot hold a run for minutes. Absent or unreadable means one interval.
func (c *BraveClient) retryAfter(header string) time.Duration {
	if secs, err := strconv.Atoi(strings.TrimSpace(header)); err == nil && secs > 0 {
		return min(time.Duration(secs)*time.Second, 5*time.Second)
	}
	return c.interval
}

// sleepCtx waits d, or returns the context's error if it ends first.
func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// parseBraveTime reads page_age, which arrives with or without a zone. An
// unreadable or absent date is nil rather than a guess.
func parseBraveTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	for _, layout := range []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}
