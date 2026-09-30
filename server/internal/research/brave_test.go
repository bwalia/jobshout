package research

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newBraveStub serves handler as the Brave API, with no spacing between
// requests unless a test sets one.
func newBraveStub(t *testing.T, handler http.HandlerFunc) *BraveClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &BraveClient{baseURL: srv.URL, apiKey: "test-key", client: srv.Client()}
}

const braveBody = `{"web":{"results":[
	{"title":"AI Hiring Tools Can Yield <strong>Racial Bias</strong>","url":"https://hai.stanford.edu/news/ai-hiring","description":"A study of <strong>AI screening</strong> tools.","page_age":"2026-06-23T09:00:00"},
	{"title":"EU AI Act: high-risk systems","url":"https://www.europarl.europa.eu/ai-act","description":"Obligations for employers."},
	{"title":"no link","url":""}
]}}`

func TestBraveSearch_MapsResults(t *testing.T) {
	var got *http.Request
	c := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) {
		got = r
		_, _ = w.Write([]byte(braveBody))
	})

	out, err := c.Search(context.Background(), "  AI candidate screening bias audit  ", 50)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	if got.URL.Path != "/web/search" || got.Header.Get("X-Subscription-Token") != "test-key" {
		t.Errorf("request = %s, token %q", got.URL.Path, got.Header.Get("X-Subscription-Token"))
	}
	q := got.URL.Query()
	if q.Get("q") != "AI candidate screening bias audit" {
		t.Errorf("q = %q", q.Get("q"))
	}
	// The API refuses more than 20 a page, whatever the caller asked for.
	if q.Get("count") != "20" {
		t.Errorf("count = %q, want it capped at 20", q.Get("count"))
	}

	if len(out) != 2 {
		t.Fatalf("got %d sources, want 2 (the result with no URL is dropped)", len(out))
	}
	first := out[0]
	if first.Title != "AI Hiring Tools Can Yield Racial Bias" || first.Excerpt != "A study of AI screening tools." {
		t.Errorf("markup survived: title %q, excerpt %q", first.Title, first.Excerpt)
	}
	if first.Site != "hai.stanford.edu" {
		t.Errorf("site = %q", first.Site)
	}
	if first.PublishedAt == nil || first.PublishedAt.Format("2006-01-02") != "2026-06-23" {
		t.Errorf("published = %v, want 2026-06-23", first.PublishedAt)
	}
	// No date reported is nil, not a guess.
	if out[1].PublishedAt != nil {
		t.Errorf("an absent page_age became %v", out[1].PublishedAt)
	}
}

func TestBraveSearch_EmptyQueryRejected(t *testing.T) {
	c := newBraveStub(t, func(http.ResponseWriter, *http.Request) { t.Error("request sent for an empty query") })
	if _, err := c.Search(context.Background(), "   ", 5); err == nil {
		t.Fatal("empty query was accepted")
	}
}

// One 429 is a request to slow down, not a failure.
func TestBraveSearch_RetriesOnceWhenRateLimited(t *testing.T) {
	var calls atomic.Int32
	c := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(braveBody))
	})
	c.interval = time.Millisecond

	out, err := c.Search(context.Background(), "q", 5)
	if err != nil || len(out) != 2 {
		t.Fatalf("Search = %d sources, %v; want the retry to succeed", len(out), err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want 2", calls.Load())
	}
}

func TestBraveSearch_StillRateLimitedIsAnError(t *testing.T) {
	var calls atomic.Int32
	c := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	c.interval = time.Millisecond

	if _, err := c.Search(context.Background(), "q", 5); err == nil || !strings.Contains(err.Error(), "429") {
		t.Fatalf("err = %v, want HTTP 429", err)
	}
	if calls.Load() != 2 {
		t.Errorf("calls = %d, want exactly one retry", calls.Load())
	}
}

// A rejected key is the one failure someone can fix, so the error names it.
func TestBraveSearch_BadKeyNamesTheSetting(t *testing.T) {
	// What the API really sends for a bad key: 422, not 401.
	c := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"SUBSCRIPTION_TOKEN_INVALID","detail":"The provided subscription token is invalid.","status":422},"type":"ErrorResponse"}`))
	})
	_, err := c.Search(context.Background(), "q", 5)
	if err == nil || !strings.Contains(err.Error(), "BRAVE_SEARCH_API_KEY") {
		t.Fatalf("err = %v, want it to name BRAVE_SEARCH_API_KEY", err)
	}
	if strings.Contains(err.Error(), "test-key") {
		t.Error("the error leaks the key")
	}
}

// Any other refusal carries the API's own reason, not just a status.
func TestBraveSearch_OtherErrorsCarryTheReason(t *testing.T) {
	c := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_, _ = w.Write([]byte(`{"error":{"code":"VALIDATION","detail":"Unable to validate request parameter(s)","status":422}}`))
	})
	_, err := c.Search(context.Background(), "q", 5)
	if err == nil || !strings.Contains(err.Error(), "VALIDATION") || strings.Contains(err.Error(), "BRAVE_SEARCH_API_KEY") {
		t.Fatalf("err = %v, want the API's reason and no key hint", err)
	}
}

// The entry plan allows one request a second. Concurrent searches must go out
// spaced, one at a time, or all but the first are refused.
func TestBraveSearch_SpacesConcurrentRequests(t *testing.T) {
	var (
		mu    sync.Mutex
		times []time.Time
	)
	c := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	})
	c.interval = 60 * time.Millisecond

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = c.Search(context.Background(), "q", 5)
		}()
	}
	wg.Wait()

	if len(times) != 4 {
		t.Fatalf("requests = %d, want 4", len(times))
	}
	for i := 1; i < len(times); i++ {
		if gap := times[i].Sub(times[i-1]); gap < 50*time.Millisecond {
			t.Errorf("requests %d and %d were %v apart, want at least the interval", i-1, i, gap)
		}
	}
}

// Waiting for a slot must not outlive the run.
func TestBraveSearch_PacingRespectsCancellation(t *testing.T) {
	c := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"web":{"results":[]}}`))
	})
	c.interval = time.Hour
	if _, err := c.Search(context.Background(), "q", 5); err != nil {
		t.Fatalf("first search: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if _, err := c.Search(ctx, "q", 5); err == nil {
		t.Fatal("second search ignored its context while waiting for a slot")
	}
}

func TestParseBraveTime(t *testing.T) {
	for in, want := range map[string]string{
		"2026-06-23T09:00:00":  "2026-06-23",
		"2026-06-23T09:00:00Z": "2026-06-23",
		"2026-06-23":           "2026-06-23",
	} {
		if got := parseBraveTime(in); got == nil || got.Format("2006-01-02") != want {
			t.Errorf("parseBraveTime(%q) = %v, want %s", in, got, want)
		}
	}
	for _, in := range []string{"", "3 days ago", "June 2026"} {
		if got := parseBraveTime(in); got != nil {
			t.Errorf("parseBraveTime(%q) = %v, want nil", in, got)
		}
	}
}

// Web search is added in front of the others, and only with a key.
func TestWithWebSearch(t *testing.T) {
	base := func() *Client { return New(zap.NewNop(), "") }

	if got := base().WithWebSearch("  ").SearchBackends(); strings.Join(got, ",") != "hackernews,arxiv" {
		t.Errorf("blank key changed the backends: %v", got)
	}
	if got := base().WithWebSearch("key").SearchBackends(); strings.Join(got, ",") != "web,hackernews,arxiv" {
		t.Errorf("backends = %v, want web first", got)
	}
}

// A subject the engineering backends do not cover: the web result leads, and
// a web backend that is down costs its results, not the search.
func TestClientSearch_WebLeadsAndItsFailureIsSurvivable(t *testing.T) {
	web := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(braveBody)) })
	hn := newHNStub(t, `{"hits":[{"objectID":"1","title":"Show HN: a speech model","url":"https://example.com/speech","points":50}]}`)

	c := NewWith(nil, []Searcher{web, hn}, nil, zap.NewNop())
	got, err := c.Search(context.Background(), "AI hiring bias", 5)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 3 || got[0].Site != "hai.stanford.edu" {
		t.Fatalf("got %d results led by %q, want 3 led by the web result", len(got), got[0].Site)
	}

	down := newBraveStub(t, func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })
	c = NewWith(nil, []Searcher{down, hn}, nil, zap.NewNop())
	got, err = c.Search(context.Background(), "AI hiring bias", 5)
	if err != nil || len(got) != 1 {
		t.Fatalf("with the web backend down: %d results, %v; want Hacker News's one", len(got), err)
	}
}
