package research

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newJinaStub serves a canned Reader envelope and returns a fetcher pointed at
// it. body is written verbatim so a test can supply a malformed payload.
func newJinaStub(t *testing.T, status int, body string) *JinaFetcher {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return &JinaFetcher{baseURL: srv.URL + "/", client: srv.Client()}
}

func TestJinaFetch_Success(t *testing.T) {
	f := newJinaStub(t, http.StatusOK, `{
		"code": 200,
		"data": {
			"title": "Writing a Kubernetes Operator",
			"description": "A walkthrough",
			"url": "https://metalbear.co/blog/writing-a-kubernetes-operator/",
			"content": "# Writing a Kubernetes Operator\n\nAn operator is...",
			"httpStatus": 200
		}
	}`)

	doc, err := f.Fetch(context.Background(), "https://metalbear.co/blog/writing-a-kubernetes-operator/")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if doc.Title != "Writing a Kubernetes Operator" {
		t.Errorf("got title %q", doc.Title)
	}
	if !strings.Contains(doc.Text, "An operator is") {
		t.Errorf("got text %q, want the article body", doc.Text)
	}
	if doc.Site != "metalbear.co" {
		t.Errorf("got site %q, want metalbear.co", doc.Site)
	}
	if doc.FetchedAt.IsZero() {
		t.Error("FetchedAt was not stamped")
	}
}

// This is the case that makes citation verification trustworthy. Reader answers
// 200 for a page that itself 404s and hands back the site's not-found page as
// perfectly readable content. Treating that as a document would let the agent
// "verify" a claim against a page that is not the cited one.
func TestJinaFetch_TargetNotFoundIsAnError(t *testing.T) {
	f := newJinaStub(t, http.StatusOK, `{
		"code": 200,
		"data": {
			"title": "404 Page not found",
			"url": "https://kubernetes.io/blog/nope",
			"content": "404 Page not found. The page you requested does not exist. Try the documentation home, or search the site. Kubernetes is an open source system for automating deployment...",
			"httpStatus": 404
		}
	}`)

	_, err := f.Fetch(context.Background(), "https://kubernetes.io/blog/nope")
	if err == nil {
		t.Fatal("Fetch accepted a page whose target returned 404")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("error %q does not mention the target status", err)
	}
}

func TestJinaFetch_UnresolvableDomainSurfacesReason(t *testing.T) {
	f := newJinaStub(t, http.StatusUnprocessableEntity, `{
		"data": null,
		"code": 422,
		"status": 42203,
		"message": "Domain 'nope.invalid' could not be resolved",
		"readableMessage": "SubmittedDataMalformedError: Domain 'nope.invalid' could not be resolved"
	}`)

	_, err := f.Fetch(context.Background(), "https://nope.invalid/")
	if err == nil {
		t.Fatal("Fetch accepted an unresolvable domain")
	}
	// The distinction between a dead citation and a rate limit matters to
	// whoever reads the run, so the backend's own wording is preserved.
	if !strings.Contains(err.Error(), "could not be resolved") {
		t.Errorf("error %q dropped the reader's explanation", err)
	}
}

func TestJinaFetch_EmptyContentIsAnError(t *testing.T) {
	f := newJinaStub(t, http.StatusOK, `{
		"code": 200,
		"data": {"title": "Empty", "url": "https://example.com/x", "content": "   ", "httpStatus": 200}
	}`)

	if _, err := f.Fetch(context.Background(), "https://example.com/x"); err == nil {
		t.Fatal("Fetch accepted a page with no extractable text")
	}
}

func TestJinaFetch_RejectsNonHTTPScheme(t *testing.T) {
	f := newJinaStub(t, http.StatusOK, `{}`)
	if _, err := f.Fetch(context.Background(), "file:///etc/passwd"); err == nil {
		t.Fatal("Fetch accepted a file:// URL")
	}
}

func TestJinaFetch_PrefersResolvedURL(t *testing.T) {
	// Citing the page a shortener resolved to is more useful than citing the
	// shortener, which may not outlive the article.
	f := newJinaStub(t, http.StatusOK, `{
		"code": 200,
		"data": {
			"title": "Real page",
			"url": "https://kubernetes.io/blog/real",
			"content": "body text here",
			"httpStatus": 200
		}
	}`)

	doc, err := f.Fetch(context.Background(), "https://bit.ly/abc")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if doc.URL != "https://kubernetes.io/blog/real" {
		t.Errorf("got URL %q, want the resolved page", doc.URL)
	}
	if doc.Site != "kubernetes.io" {
		t.Errorf("got site %q, want the resolved host", doc.Site)
	}
}

// newHNStub serves a canned Algolia envelope.
func newHNStub(t *testing.T, body string) *HNClient {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)

	return &HNClient{baseURL: srv.URL, client: srv.Client()}
}

func TestHNSearch_MapsHits(t *testing.T) {
	c := newHNStub(t, `{"hits":[
		{"objectID":"35081033","title":"Writing a Kubernetes Operator","url":"https://metalbear.co/blog/op/","points":169,"num_comments":74,"created_at":"2023-03-09T13:41:05Z"}
	]}`)

	got, err := c.Search(context.Background(), "kubernetes operator", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1", len(got))
	}
	if got[0].URL != "https://metalbear.co/blog/op/" {
		t.Errorf("got URL %q", got[0].URL)
	}
	if got[0].PublishedAt == nil || got[0].PublishedAt.Year() != 2023 {
		t.Errorf("got PublishedAt %v, want the 2023 submission date", got[0].PublishedAt)
	}
}

func TestHNSearch_TextPostFallsBackToItemPage(t *testing.T) {
	// Ask HN threads carry no external URL but are often where the real detail
	// about a release lives, so they stay citable via their item page.
	c := newHNStub(t, `{"hits":[
		{"objectID":"12345","title":"Ask HN: how are you running Postgres?","url":"","points":80,"created_at":"2026-08-01T10:00:00Z","story_text":"We run..."}
	]}`)

	got, err := c.Search(context.Background(), "postgres", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1", len(got))
	}
	if got[0].URL != hnItemURL+"12345" {
		t.Errorf("got URL %q, want the HN item page", got[0].URL)
	}
}

func TestHNSearch_EmptyQueryRejected(t *testing.T) {
	c := newHNStub(t, `{"hits":[]}`)
	if _, err := c.Search(context.Background(), "  ", 10); err == nil {
		t.Fatal("Search accepted an empty query")
	}
}

func TestHNList_CarriesPointsAsScore(t *testing.T) {
	c := newHNStub(t, `{"hits":[
		{"objectID":"1","title":"A","url":"https://a.com/1","points":412,"created_at":"2026-08-13T09:00:00Z"},
		{"objectID":"2","title":"B","url":"https://b.com/2","points":88,"created_at":"2026-08-13T08:00:00Z"}
	]}`)

	got, err := c.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2", len(got))
	}
	if got[0].Score != 412 {
		t.Errorf("got score %d, want 412", got[0].Score)
	}
	if got[0].Channel != "hackernews" {
		t.Errorf("got channel %q, want hackernews", got[0].Channel)
	}
}

func TestHNGet_NonOKStatusIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	t.Cleanup(srv.Close)
	c := &HNClient{baseURL: srv.URL, client: srv.Client()}

	if _, err := c.Search(context.Background(), "anything", 10); err == nil {
		t.Fatal("Search accepted a 429 response")
	}
}

const rssSample = `<?xml version="1.0" encoding="utf-8"?>
<rss version="2.0"><channel>
  <title>Kubernetes Blog</title>
  <item>
    <title>Kubernetes v1.34 Released</title>
    <link>https://kubernetes.io/blog/2026/08/10/v134/</link>
    <description>&lt;p&gt;The release includes &lt;b&gt;gateway&lt;/b&gt; improvements.&lt;/p&gt;</description>
    <pubDate>Mon, 10 Aug 2026 09:00:00 +0000</pubDate>
  </item>
  <item>
    <title>Ancient Post</title>
    <link>https://kubernetes.io/blog/2019/01/01/old/</link>
    <description>Old news</description>
    <pubDate>Tue, 01 Jan 2019 09:00:00 +0000</pubDate>
  </item>
</channel></rss>`

const atomSample = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>The Go Blog</title>
  <entry>
    <title>Go 1.26 is released</title>
    <link href="https://go.dev/blog/go1.26" rel="alternate" type="text/html"/>
    <link href="https://go.dev/blog/feed.atom" rel="self" type="application/atom+xml"/>
    <summary>Go 1.26 adds...</summary>
    <published>2026-08-11T12:00:00Z</published>
  </entry>
</feed>`

func TestParseFeed_RSS(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)

	got, err := parseFeed([]byte(rssSample), "kubernetes", now)
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}

	// The 2019 post is outside feedMaxAge and must not present as trending.
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1 after the age cutoff: %+v", len(got), got)
	}
	item := got[0]
	if item.URL != "https://kubernetes.io/blog/2026/08/10/v134/" {
		t.Errorf("got URL %q", item.URL)
	}
	if item.Channel != "rss:kubernetes" {
		t.Errorf("got channel %q, want rss:kubernetes", item.Channel)
	}
	if strings.Contains(item.Excerpt, "<") {
		t.Errorf("excerpt %q still contains markup", item.Excerpt)
	}
	if !strings.Contains(item.Excerpt, "gateway") {
		t.Errorf("excerpt %q lost the description text", item.Excerpt)
	}
}

func TestParseFeed_Atom(t *testing.T) {
	now := time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC)

	got, err := parseFeed([]byte(atomSample), "golang", now)
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	// rel="self" points at the feed document; picking it would cite the feed
	// rather than the article.
	if got[0].URL != "https://go.dev/blog/go1.26" {
		t.Errorf("got URL %q, want the rel=alternate link", got[0].URL)
	}
}

func TestParseFeed_UndatedEntriesAreKept(t *testing.T) {
	// Some feeds omit dates entirely. Dropping them would silently remove
	// those sources from every sweep.
	const noDate = `<rss version="2.0"><channel>
	  <item><title>Undated</title><link>https://example.com/x</link></item>
	</channel></rss>`

	got, err := parseFeed([]byte(noDate), "example", time.Now())
	if err != nil {
		t.Fatalf("parseFeed returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want the undated entry kept", len(got))
	}
	if got[0].PublishedAt != nil {
		t.Error("an unknown date should stay nil rather than defaulting")
	}
}

func TestParseFeed_EmptyFeedIsAnError(t *testing.T) {
	const empty = `<rss version="2.0"><channel><title>Nothing</title></channel></rss>`
	if _, err := parseFeed([]byte(empty), "example", time.Now()); err == nil {
		t.Fatal("parseFeed accepted a feed with no items")
	}
}

func TestParseFeedTime(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool // whether a time is expected
	}{
		{"RFC1123Z", "Mon, 10 Aug 2026 09:00:00 +0000", true},
		{"RFC3339", "2026-08-11T12:00:00Z", true},
		{"single digit day", "Mon, 1 Jan 2026 09:00:00 -0700", true},
		{"date only", "2026-08-11", true},
		{"empty", "", false},
		{"garbage", "last tuesday", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseFeedTime(tt.in)
			if tt.want && got == nil {
				t.Errorf("parseFeedTime(%q) = nil, want a time", tt.in)
			}
			if !tt.want && got != nil {
				t.Errorf("parseFeedTime(%q) = %v, want nil", tt.in, got)
			}
		})
	}
}

func TestStripTags(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"removes markup", "<p>Hello <b>world</b></p>", "Hello world"},
		{"collapses whitespace", "<p>a</p>\n\n   <p>b</p>", "a b"},
		{"plain text untouched", "no markup here", "no markup here"},
		{"unclosed tag", "<p>text", "text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stripTags(tt.in); got != tt.want {
				t.Errorf("stripTags(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestFeedClient_AllFeedsFailingIsAnError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)

	c := NewFeedClient([]Feed{{Name: "broken", URL: srv.URL}})
	c.client = srv.Client()

	if _, err := c.List(context.Background(), 10); err == nil {
		t.Fatal("List reported success when every feed failed")
	}
}

func TestFeedClient_PartialFailureStillReturnsItems(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(atomSample))
	}))
	t.Cleanup(good.Close)
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(bad.Close)

	c := NewFeedClient([]Feed{
		{Name: "good", URL: good.URL},
		{Name: "dead", URL: bad.URL},
	})
	c.client = good.Client()
	c.now = func() time.Time { return time.Date(2026, 8, 13, 0, 0, 0, 0, time.UTC) }

	got, err := c.List(context.Background(), 10)
	if err != nil {
		t.Fatalf("List failed despite one healthy feed: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1 from the healthy feed", len(got))
	}
}

func TestArxivQuery_ParsesEntries(t *testing.T) {
	const arxivSample = `<?xml version="1.0" encoding="UTF-8"?>
	<feed xmlns="http://www.w3.org/2005/Atom">
	  <entry>
	    <id>http://arxiv.org/abs/2608.12304v1</id>
	    <title>Retrieval-Augmented Diagnostics</title>
	    <link href="https://arxiv.org/abs/2608.12304v1" rel="alternate" type="text/html"/>
	    <link href="https://arxiv.org/pdf/2608.12304v1" rel="related" type="application/pdf"/>
	    <summary>Dynamic Master Logic provides a hierarchical framework.</summary>
	    <published>2026-08-12T17:50:39Z</published>
	  </entry>
	</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(arxivSample))
	}))
	t.Cleanup(srv.Close)

	c := &ArxivClient{baseURL: srv.URL, categories: ArxivCategories, client: srv.Client()}

	got, err := c.Search(context.Background(), "retrieval augmented generation", 5)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1", len(got))
	}
	// rel="related" is the PDF. An abstract page is what a reader should be
	// sent to, and what a verification pass can actually extract text from.
	if got[0].URL != "https://arxiv.org/abs/2608.12304v1" {
		t.Errorf("got URL %q, want the abstract page", got[0].URL)
	}
}

func TestArxivList_TagsChannel(t *testing.T) {
	const sample = `<feed xmlns="http://www.w3.org/2005/Atom">
	  <entry>
	    <title>A paper</title>
	    <link href="https://arxiv.org/abs/1" rel="alternate"/>
	    <published>2026-08-12T17:50:39Z</published>
	  </entry>
	</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(sample))
	}))
	t.Cleanup(srv.Close)

	c := &ArxivClient{baseURL: srv.URL, categories: ArxivCategories, client: srv.Client()}

	got, err := c.List(context.Background(), 5)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	if got[0].Channel != "arxiv" {
		t.Errorf("got channel %q, want arxiv", got[0].Channel)
	}
}
