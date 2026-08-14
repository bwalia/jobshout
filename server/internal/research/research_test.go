package research

import (
	"context"
	"testing"
	"time"
)

func TestSiteOf(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"strips www", "https://www.kubernetes.io/blog/", "kubernetes.io"},
		{"lowercases host", "https://Blog.Cloudflare.COM/rss/", "blog.cloudflare.com"},
		{"keeps subdomain", "https://news.ycombinator.com/item?id=1", "news.ycombinator.com"},
		{"unparseable is empty", "not a url", ""},
		{"no host is empty", "file:///etc/passwd", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := siteOf(tt.in); got != tt.want {
				t.Errorf("siteOf(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestValidateURL(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		wantErr bool
	}{
		{"https ok", "https://example.com/a", false},
		{"http ok", "http://example.com/a", false},
		{"empty rejected", "  ", true},
		{"file scheme rejected", "file:///etc/passwd", true},
		{"javascript scheme rejected", "javascript:alert(1)", true},
		{"no host rejected", "https://", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateURL(tt.in)
			if tt.wantErr && err == nil {
				t.Errorf("validateURL(%q) = nil error, want error", tt.in)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("validateURL(%q) = %v, want nil", tt.in, err)
			}
		})
	}
}

func TestCanonicalURL_TreatsVariantsAsOneSource(t *testing.T) {
	// These are all the same page. If they canonicalise differently the agent
	// reads and cites the same article several times.
	variants := []string{
		"https://kubernetes.io/blog/post/",
		"https://www.kubernetes.io/blog/post",
		"https://kubernetes.io/blog/post?utm_source=hn&utm_medium=social",
		"https://kubernetes.io/blog/post#section",
	}
	want := canonicalURL(variants[0])
	if want == "" {
		t.Fatal("canonicalURL returned empty for a valid URL")
	}
	for _, v := range variants[1:] {
		if got := canonicalURL(v); got != want {
			t.Errorf("canonicalURL(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestDedupeSources_KeepsFirstSeenOrder(t *testing.T) {
	in := []Source{
		{URL: "https://a.com/1", Title: "first"},
		{URL: "https://b.com/2", Title: "second"},
		{URL: "https://www.a.com/1/", Title: "duplicate of first"},
		{URL: "bad url", Title: "dropped"},
		{URL: "https://c.com/3", Title: "third"},
	}

	got := dedupeSources(in)

	want := []string{"first", "second", "third"}
	if len(got) != len(want) {
		t.Fatalf("got %d sources, want %d: %+v", len(got), len(want), got)
	}
	for i, w := range want {
		if got[i].Title != w {
			t.Errorf("source %d = %q, want %q", i, got[i].Title, w)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"under limit unchanged", "short", 10, "short"},
		{"at limit unchanged", "exactly10!", 10, "exactly10!"},
		{"over limit gets ellipsis", "abcdefghijk", 5, "abcde…"},
		{"counts runes not bytes", "héllo wörld", 5, "héllo…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := truncate(tt.in, tt.n); got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
		})
	}
}

func TestClampLimit(t *testing.T) {
	tests := []struct {
		name string
		in   int
		want int
	}{
		{"zero becomes default", 0, DefaultLimit},
		{"negative becomes default", -5, DefaultLimit},
		{"in range preserved", 7, 7},
		{"above max clamped", 999, maxLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clampLimit(tt.in); got != tt.want {
				t.Errorf("clampLimit(%d) = %d, want %d", tt.in, got, tt.want)
			}
		})
	}
}

func TestInterleaveSources_RoundRobinsAcrossBackends(t *testing.T) {
	buckets := [][]Source{
		{{URL: "https://a.com/1"}, {URL: "https://a.com/2"}, {URL: "https://a.com/3"}},
		{{URL: "https://b.com/1"}},
		{{URL: "https://c.com/1"}, {URL: "https://c.com/2"}},
	}

	got := interleaveSources(buckets)

	want := []string{
		"https://a.com/1", "https://b.com/1", "https://c.com/1",
		"https://a.com/2", "https://c.com/2",
		"https://a.com/3",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sources, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].URL != w {
			t.Errorf("position %d = %q, want %q", i, got[i].URL, w)
		}
	}
}

func TestInterleaveSources_Empty(t *testing.T) {
	if got := interleaveSources(nil); len(got) != 0 {
		t.Errorf("interleaveSources(nil) = %v, want empty", got)
	}
}

// stubSearcher / stubLister / stubFetcher back the Client tests without a
// network. They are deliberately dumb: the Client's job is fan-out, merge and
// failure policy, and a real backend would only obscure whether that works.

type stubSearcher struct {
	name    string
	sources []Source
	err     error
}

func (s stubSearcher) Name() string { return s.name }
func (s stubSearcher) Search(context.Context, string, int) ([]Source, error) {
	return s.sources, s.err
}

type stubLister struct {
	name  string
	items []TrendingItem
	err   error
}

func (s stubLister) Name() string { return s.name }
func (s stubLister) List(context.Context, int) ([]TrendingItem, error) {
	return s.items, s.err
}

type stubFetcher struct {
	doc *Document
	err error
}

func (s stubFetcher) Fetch(context.Context, string) (*Document, error) { return s.doc, s.err }

func TestClientSearch_MergesAndDedupes(t *testing.T) {
	c := NewWith(nil,
		[]Searcher{
			stubSearcher{name: "one", sources: []Source{
				{URL: "https://shared.com/x"},
				{URL: "https://one.com/a"},
			}},
			stubSearcher{name: "two", sources: []Source{
				{URL: "https://www.shared.com/x/"}, // same page as above
				{URL: "https://two.com/b"},
			}},
		}, nil, nil)

	got, err := c.Search(context.Background(), "anything", 10)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d sources, want 3 after de-duplication: %+v", len(got), got)
	}
}

func TestClientSearch_PartialFailureStillReturnsResults(t *testing.T) {
	// One backend being down must degrade coverage, not the feature — an
	// article can be written from the sources that did come back.
	c := NewWith(nil,
		[]Searcher{
			stubSearcher{name: "healthy", sources: []Source{{URL: "https://ok.com/a"}}},
			stubSearcher{name: "broken", err: context.DeadlineExceeded},
		}, nil, nil)

	got, err := c.Search(context.Background(), "anything", 10)
	if err != nil {
		t.Fatalf("Search returned error despite a healthy backend: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d sources, want 1", len(got))
	}
}

func TestClientSearch_TotalFailureIsAnError(t *testing.T) {
	// Every backend failing is not "no results found" — reporting it as an
	// empty list would let the pipeline write an uncited article and call it a
	// success.
	c := NewWith(nil,
		[]Searcher{
			stubSearcher{name: "broken-1", err: context.DeadlineExceeded},
			stubSearcher{name: "broken-2", err: context.DeadlineExceeded},
		}, nil, nil)

	if _, err := c.Search(context.Background(), "anything", 10); err == nil {
		t.Fatal("Search returned nil error when every backend failed")
	}
}

func TestClientSearch_EmptyQueryRejected(t *testing.T) {
	c := NewWith(nil, []Searcher{stubSearcher{name: "one"}}, nil, nil)
	if _, err := c.Search(context.Background(), "   ", 10); err == nil {
		t.Fatal("Search accepted an empty query")
	}
}

func TestClientSearch_RespectsLimit(t *testing.T) {
	many := make([]Source, 0, 20)
	for i := 0; i < 20; i++ {
		many = append(many, Source{URL: "https://example.com/" + string(rune('a'+i))})
	}
	c := NewWith(nil, []Searcher{stubSearcher{name: "one", sources: many}}, nil, nil)

	got, err := c.Search(context.Background(), "anything", 5)
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(got) != 5 {
		t.Errorf("got %d sources, want 5", len(got))
	}
}

func TestClientTrending_DedupesAcrossChannels(t *testing.T) {
	// The same article routinely trends on HN and appears in the publisher's
	// own feed. It is one candidate topic, not two.
	c := NewWith(nil, nil, []Lister{
		stubLister{name: "hackernews", items: []TrendingItem{
			{Source: Source{URL: "https://kubernetes.io/blog/x"}, Channel: "hackernews", Score: 200},
		}},
		stubLister{name: "rss", items: []TrendingItem{
			{Source: Source{URL: "https://www.kubernetes.io/blog/x/"}, Channel: "rss:kubernetes"},
			{Source: Source{URL: "https://cncf.io/blog/y"}, Channel: "rss:cncf"},
		}},
	}, nil)

	got, err := c.Trending(context.Background(), 10)
	if err != nil {
		t.Fatalf("Trending returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d items, want 2 after de-duplication: %+v", len(got), got)
	}
	// Interleaving puts the first backend's copy first, so the surviving entry
	// keeps the HN score rather than the feed's zero.
	if got[0].Score != 200 {
		t.Errorf("kept the feed copy (score %d) over the ranked HN copy", got[0].Score)
	}
}

func TestClientTrending_KeepsBestScoreRegardlessOfChannelOrder(t *testing.T) {
	// Same as the test above but with the unranked channel listed first. The
	// popularity signal must survive either way, or Phase 2's ranking silently
	// depends on the order the backends happen to be wired in.
	c := NewWith(nil, nil, []Lister{
		stubLister{name: "rss", items: []TrendingItem{
			{Source: Source{URL: "https://www.kubernetes.io/blog/x/"}, Channel: "rss:kubernetes"},
		}},
		stubLister{name: "hackernews", items: []TrendingItem{
			{Source: Source{URL: "https://kubernetes.io/blog/x"}, Channel: "hackernews", Score: 200},
		}},
	}, nil)

	got, err := c.Trending(context.Background(), 10)
	if err != nil {
		t.Fatalf("Trending returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1 after de-duplication", len(got))
	}
	if got[0].Score != 200 {
		t.Errorf("got score %d, want the HN score of 200 merged in", got[0].Score)
	}
}

func TestClientTrending_TotalFailureIsAnError(t *testing.T) {
	c := NewWith(nil, nil, []Lister{
		stubLister{name: "broken", err: context.DeadlineExceeded},
	}, nil)

	if _, err := c.Trending(context.Background(), 10); err == nil {
		t.Fatal("Trending returned nil error when every backend failed")
	}
}

func TestClientFetch_NoFetcherConfigured(t *testing.T) {
	c := NewWith(nil, nil, nil, nil)
	if _, err := c.Fetch(context.Background(), "https://example.com"); err == nil {
		t.Fatal("Fetch succeeded with no fetcher configured")
	}
}

func TestClientFetch_DelegatesToFetcher(t *testing.T) {
	want := &Document{Source: Source{URL: "https://example.com/a"}, Text: "body"}
	c := NewWith(stubFetcher{doc: want}, nil, nil, nil)

	got, err := c.Fetch(context.Background(), "https://example.com/a")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if got.Text != "body" {
		t.Errorf("got text %q, want %q", got.Text, "body")
	}
}

func TestSortByPublished_NewestFirstUndatedLast(t *testing.T) {
	newer := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	older := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	items := []TrendingItem{
		{Source: Source{URL: "https://a.com/undated"}},
		{Source: Source{URL: "https://a.com/older", PublishedAt: &older}},
		{Source: Source{URL: "https://a.com/newer", PublishedAt: &newer}},
	}

	sortByPublished(items)

	want := []string{"https://a.com/newer", "https://a.com/older", "https://a.com/undated"}
	for i, w := range want {
		if items[i].URL != w {
			t.Errorf("position %d = %q, want %q", i, items[i].URL, w)
		}
	}
}
