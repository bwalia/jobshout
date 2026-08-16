package research

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const redditFeed = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>reddit.com: search results</title>
  <entry>
    <title>Hot take: Gateway API is not ready for production ingress</title>
    <link href="https://www.reddit.com/r/kubernetes/comments/abc123/hot_take_gateway_api/" rel="alternate"/>
    <content>We migrated 200 Ingress objects and hit three separate bugs.</content>
    <updated>2026-08-10T09:00:00+00:00</updated>
  </entry>
  <entry>
    <title>r/kubernetes</title>
    <link href="https://www.reddit.com/r/kubernetes/" rel="alternate"/>
    <updated>2026-08-10T09:00:00+00:00</updated>
  </entry>
</feed>`

func newRedditStub(t *testing.T, status int, body string) *RedditClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return &RedditClient{baseURL: srv.URL, subreddits: DefaultSubreddits, client: srv.Client()}
}

func TestRedditSearch_ReturnsThreads(t *testing.T) {
	c := newRedditStub(t, http.StatusOK, redditFeed)

	got, err := c.Search(context.Background(), "gateway api", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// The listing entry is not a thread and must not be citable — a citation
	// should point at a discussion, not at a subreddit's front page.
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1 thread: %+v", len(got), got)
	}
	if !strings.Contains(got[0].Title, "Gateway API is not ready") {
		t.Errorf("got title %q", got[0].Title)
	}
	if got[0].Site != "reddit.com" {
		t.Errorf("site = %q, want reddit.com", got[0].Site)
	}
}

// Rate limiting is the normal failure here, and it has to be legible: the fix
// is "wait", not "search for something else".
func TestRedditSearch_RateLimitIsExplained(t *testing.T) {
	c := newRedditStub(t, http.StatusTooManyRequests, "")

	_, err := c.Search(context.Background(), "anything", 10)
	if err == nil {
		t.Fatal("Search succeeded against a rate-limited feed")
	}
	if !strings.Contains(err.Error(), "rate limited") {
		t.Errorf("error %q does not name the cause", err)
	}
}

func TestRedditSearch_BlockedIsExplained(t *testing.T) {
	c := newRedditStub(t, http.StatusForbidden, "")

	_, err := c.Search(context.Background(), "anything", 10)
	if err == nil {
		t.Fatal("Search succeeded against a 403")
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error %q does not report the status", err)
	}
}

func TestRedditSearch_EmptyQueryRejected(t *testing.T) {
	c := newRedditStub(t, http.StatusOK, redditFeed)
	if _, err := c.Search(context.Background(), "  ", 10); err == nil {
		t.Fatal("Search accepted an empty query")
	}
}

func TestRedditList_TagsChannel(t *testing.T) {
	c := newRedditStub(t, http.StatusOK, redditFeed)

	got, err := c.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	if got[0].Channel != "reddit" {
		t.Errorf("channel = %q, want reddit", got[0].Channel)
	}
}

// A throttled Reddit must not take the research down with it — it is one
// backend among several, and contributing nothing is an acceptable outcome.
func TestClientSearch_SurvivesRedditBeingThrottled(t *testing.T) {
	throttled := newRedditStub(t, http.StatusTooManyRequests, "")
	c := NewWith(nil, []Searcher{
		stubSearcher{name: "hackernews", sources: []Source{{URL: "https://a.com/1", Title: "From HN"}}},
		throttled,
	}, nil, nil)

	got, err := c.Search(context.Background(), "anything", 10)
	if err != nil {
		t.Fatalf("Search failed because Reddit was throttled: %v", err)
	}
	if len(got) != 1 || got[0].Title != "From HN" {
		t.Errorf("got %+v, want the healthy backend's result", got)
	}
}

func TestIsRedditThread(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"thread", "https://www.reddit.com/r/kubernetes/comments/abc/title/", true},
		{"subreddit listing", "https://www.reddit.com/r/kubernetes/", false},
		{"user page", "https://www.reddit.com/user/someone", false},
		{"not reddit", "https://kubernetes.io/blog/", false},
		{"unparseable", "://nope", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRedditThread(tt.in); got != tt.want {
				t.Errorf("isRedditThread(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

const redlibPage = `<html><body>
<div class="post">
  <a href="/r/kubernetes/comments/abc123/migration_from_nginx/?sort=top">Migration from ingress-nginx to cilium: good/bad/ugly</a>
  <a href="/r/kubernetes/comments/abc123/migration_from_nginx/#comments">45 comments</a>
</div>
<div class="post">
  <a href="/r/devops/comments/def456/gateway_api_maintainer/">Gateway API for Ingress-NGINX - a Maintainer's Perspective</a>
  <a href="/r/devops/comments/def456/gateway_api_maintainer/">my comment</a>
</div>
<a href="/r/kubernetes/">r/kubernetes</a>
</body></html>`

func TestParseRedlibThreads(t *testing.T) {
	got := parseRedlibThreads(redlibPage, 10)

	if len(got) != 2 {
		t.Fatalf("got %d threads, want 2: %+v", len(got), got)
	}

	// Citations must point at reddit.com. Mirrors are a way of reading Reddit,
	// not a place to send a reader — they come and go, and a citation pointing
	// at one rots with it.
	for _, s := range got {
		if !strings.HasPrefix(s.URL, "https://www.reddit.com/r/") {
			t.Errorf("URL %q was not rewritten back to reddit.com", s.URL)
		}
		if strings.ContainsAny(s.URL, "?#") {
			t.Errorf("URL %q kept the mirror's query parameters", s.URL)
		}
		if s.Site != "reddit.com" {
			t.Errorf("site = %q", s.Site)
		}
	}
	if !strings.Contains(got[0].Title, "Migration from ingress-nginx") {
		t.Errorf("first title = %q", got[0].Title)
	}
}

// "45 comments" and "my comment" are links to the same thread, not titles.
func TestParseRedlibThreads_SkipsNonTitleLinks(t *testing.T) {
	for _, s := range parseRedlibThreads(redlibPage, 10) {
		if strings.Contains(s.Title, "comments") || s.Title == "my comment" {
			t.Errorf("kept a non-title link as a source: %q", s.Title)
		}
	}
}

func TestParseRedlibThreads_RespectsLimit(t *testing.T) {
	if got := parseRedlibThreads(redlibPage, 1); len(got) != 1 {
		t.Errorf("got %d threads, want 1", len(got))
	}
}

// Markup we do not recognise returns nothing, so the caller falls back to
// reddit.com rather than inventing results.
func TestParseRedlibThreads_UnknownMarkupReturnsNothing(t *testing.T) {
	if got := parseRedlibThreads("<html><body><p>nothing here</p></body></html>", 10); len(got) != 0 {
		t.Errorf("got %+v, want nothing from unrecognised markup", got)
	}
}
