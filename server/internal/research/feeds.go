package research

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// DefaultFeeds are the sources trending discovery reads when no feed list is
// configured. Each was checked to be live and well-formed at the time it was
// added; a feed that starts failing is skipped with a warning rather than
// failing the sweep, so a dead entry degrades coverage instead of the feature.
//
// The list is deliberately first-party engineering blogs and release channels.
// They are where infrastructure and AI changes are announced, which is what an
// article should be grounded in — as opposed to aggregators, which mostly
// restate them a day later and would crowd the results with duplicates.
var DefaultFeeds = []Feed{
	{Name: "kubernetes", URL: "https://kubernetes.io/feed.xml"},
	{Name: "cncf", URL: "https://www.cncf.io/feed/"},
	{Name: "cloudflare", URL: "https://blog.cloudflare.com/rss/"},
	{Name: "github", URL: "https://github.blog/feed/"},
	{Name: "infoq", URL: "https://feed.infoq.com/"},
	{Name: "openai", URL: "https://openai.com/news/rss.xml"},
	{Name: "netflix-tech", URL: "https://netflixtechblog.com/feed"},
	{Name: "aws-architecture", URL: "https://aws.amazon.com/blogs/architecture/feed/"},
	{Name: "golang", URL: "https://blog.golang.org/feed.atom"},
	{Name: "lwn", URL: "https://lwn.net/headlines/newrss"},
	{Name: "hashicorp", URL: "https://www.hashicorp.com/blog/feed.xml"},
}

// Feed is one subscribed source.
type Feed struct {
	// Name is the short label used in a TrendingItem's Channel ("rss:kubernetes").
	Name string
	URL  string
}

// feedMaxBody caps a single feed read. Some feeds publish their entire archive
// — openai.com/news returns over a thousand entries — and there is no reason to
// hold all of it in memory to take the newest ten.
const feedMaxBody = 8 << 20 // 8 MB

// feedFetchTimeout bounds one feed. The sweep runs them concurrently, so this
// is the ceiling on the whole operation rather than a per-feed budget spent
// end to end.
const feedFetchTimeout = 15 * time.Second

// feedMaxAge is how far back a feed entry can be published and still count as
// trending. Feeds that publish their whole archive would otherwise contribute
// years-old posts to a "what is happening now" list.
const feedMaxAge = 30 * 24 * time.Hour

// FeedClient reads RSS and Atom feeds. It satisfies Lister only: a feed has no
// query interface, and pretending otherwise by filtering titles in-process
// would return a fraction of what is actually published on a topic.
type FeedClient struct {
	feeds  []Feed
	client *http.Client
	// now is injected so tests can assert the age filter deterministically.
	now func() time.Time
}

// NewFeedClient builds a client over the given feeds, or DefaultFeeds when the
// list is empty.
func NewFeedClient(feeds []Feed) *FeedClient {
	if len(feeds) == 0 {
		feeds = DefaultFeeds
	}
	return &FeedClient{
		feeds:  feeds,
		client: &http.Client{Timeout: feedFetchTimeout},
		now:    time.Now,
	}
}

// Name identifies this backend.
func (c *FeedClient) Name() string { return "rss" }

// List sweeps every feed concurrently and returns the newest entries across
// all of them, most recent first.
//
// limit applies to the merged result, not per feed. Feeds are read in parallel
// because they are independent network calls to eleven different hosts, and
// doing them in sequence makes the trending sweep take as long as the sum of
// the slowest.
func (c *FeedClient) List(ctx context.Context, limit int) ([]TrendingItem, error) {
	limit = clampLimit(limit)

	var (
		mu       sync.Mutex
		all      []TrendingItem
		failures []string
		wg       sync.WaitGroup
	)

	for _, f := range c.feeds {
		wg.Add(1)
		go func(f Feed) {
			defer wg.Done()
			items, err := c.readFeed(ctx, f)

			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				failures = append(failures, fmt.Sprintf("%s: %v", f.Name, err))
				return
			}
			all = append(all, items...)
		}(f)
	}
	wg.Wait()

	// Every feed failing is a real error — usually no outbound network — and
	// should not be reported as "nothing is trending". A partial failure is
	// not: ten good feeds and one 404 is still a usable answer.
	if len(all) == 0 && len(failures) > 0 {
		return nil, fmt.Errorf("research: all %d feeds failed: %s",
			len(failures), strings.Join(failures, "; "))
	}

	sortByPublished(all)
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// readFeed retrieves and parses one feed.
func (c *FeedClient) readFeed(ctx context.Context, f Feed) ([]TrendingItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	// Several feed hosts return 403 to a bare Go user agent.
	req.Header.Set("User-Agent", "JobShout-ResearchAgent/1.0 (+https://github.com/jobshout)")
	req.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml, text/xml")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBody))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return parseFeed(body, f.Name, c.now())
}

// xmlFeed models RSS 2.0 and Atom in one structure.
//
// The two formats disagree on almost every name, but they never collide: an
// RSS document has channel/item, an Atom document has entry, and a parsed
// document populates one set or the other. One struct with both sets of tags
// is smaller and less error-prone than sniffing the root element and running
// two parsers.
type xmlFeed struct {
	// RSS 2.0
	Channel struct {
		Items []xmlItem `xml:"item"`
	} `xml:"channel"`
	// Atom
	Entries []xmlItem `xml:"entry"`
}

type xmlItem struct {
	Title string `xml:"title"`
	// Links captures both formats in one field, which encoding/xml requires —
	// two fields sharing the "link" tag is a decoder error. RSS writes the URL
	// as the element's text and Atom writes it as an href attribute, so each
	// xmlLink carries whichever of the two the document used.
	Links []xmlLink `xml:"link"`
	// Description is RSS; Summary and Content are Atom.
	Description string `xml:"description"`
	Summary     string `xml:"summary"`
	Content     string `xml:"content"`
	// PubDate is RSS; Published and Updated are Atom.
	PubDate   string `xml:"pubDate"`
	Published string `xml:"published"`
	Updated   string `xml:"updated"`
}

type xmlLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	// Text is the element's character data, which is where RSS puts the URL.
	Text string `xml:",chardata"`
}

// decodeFeed parses an RSS or Atom document. Shared with the arXiv client,
// which speaks Atom over a different transport.
func decodeFeed(body []byte) (*xmlFeed, error) {
	var doc xmlFeed
	dec := xml.NewDecoder(strings.NewReader(string(body)))
	// Feeds in the wild declare charsets Go does not know natively. Passing
	// the bytes through unconverted reads a latin-1 feed with mojibake in the
	// occasional title, which is better than refusing the feed outright.
	dec.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	dec.Strict = false

	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse feed: %w", err)
	}
	return &doc, nil
}

// parseFeed decodes a feed document into trending items, dropping entries
// older than feedMaxAge.
func parseFeed(body []byte, feedName string, now time.Time) ([]TrendingItem, error) {
	doc, err := decodeFeed(body)
	if err != nil {
		return nil, err
	}

	raw := doc.Channel.Items
	if len(raw) == 0 {
		raw = doc.Entries
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("feed contains no items")
	}

	cutoff := now.Add(-feedMaxAge)
	out := make([]TrendingItem, 0, len(raw))
	for _, it := range raw {
		src := it.toSource()
		if src.URL == "" {
			continue
		}
		// An entry with no parseable date is kept: some feeds omit dates
		// entirely, and dropping them would silently remove those sources.
		if src.PublishedAt != nil && src.PublishedAt.Before(cutoff) {
			continue
		}
		out = append(out, TrendingItem{
			Source: src,
			// Feeds publish no popularity signal. Leaving this 0 rather than
			// inventing one keeps Score comparable only within a channel,
			// which is what the field documents.
			Score:   0,
			Channel: "rss:" + feedName,
		})
	}
	return out, nil
}

// toSource normalises one entry across the two formats.
func (i xmlItem) toSource() Source {
	link := ""
	for _, l := range i.Links {
		// Skip the links that are not the article: Atom uses rel="self" for
		// the feed document and rel="related" for alternate representations,
		// which on arXiv is the PDF rather than the abstract page.
		if l.Rel != "" && l.Rel != "alternate" {
			continue
		}
		if href := strings.TrimSpace(l.Href); href != "" {
			link = href
			break
		}
		if text := strings.TrimSpace(l.Text); text != "" {
			link = text
			break
		}
	}
	if link == "" {
		return Source{}
	}

	excerpt := firstNonEmpty(i.Description, i.Summary, i.Content)

	return Source{
		URL:   link,
		Title: strings.TrimSpace(i.Title),
		Site:  siteOf(link),
		// Atom's <updated> is a fallback: an entry that has only ever been
		// published carries no <published> in some generators.
		PublishedAt: parseFeedTime(firstNonEmpty(i.PubDate, i.Published, i.Updated)),
		Excerpt:     truncate(stripTags(excerpt), 300),
	}
}

// feedTimeLayouts covers the formats feeds actually emit. RSS specifies
// RFC822 and Atom specifies RFC3339, but generators are inconsistent about
// zone format and second precision, so each variant is tried in turn.
var feedTimeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC3339,
	"2006-01-02T15:04:05Z0700",
	"2006-01-02T15:04:05",
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"Mon, 2 Jan 2006 15:04:05 MST",
	"2006-01-02",
}

// parseFeedTime returns nil rather than a zero time when nothing parses, so an
// unknown date stays distinguishable from the epoch.
func parseFeedTime(s string) *time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range feedTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return &t
		}
	}
	return nil
}

// sortByPublished orders items newest first. Items with no date sort last:
// they cannot be shown to be current, and this list is about what is current.
func sortByPublished(items []TrendingItem) {
	// Insertion sort by publication date. The slice is bounded by maxLimit
	// times the feed count, so an allocation-free sort beats pulling in a
	// comparator closure for a list this size.
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && publishedAfter(items[j], items[j-1]); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}

func publishedAfter(a, b TrendingItem) bool {
	if a.PublishedAt == nil {
		return false
	}
	if b.PublishedAt == nil {
		return true
	}
	return a.PublishedAt.After(*b.PublishedAt)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}

// stripTags removes HTML markup from a feed excerpt. Feed descriptions are
// routinely full HTML documents, and the excerpt exists to be read by a model
// deciding whether a source is worth fetching — tags are pure noise there.
func stripTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	depth := 0
	for _, r := range s {
		switch {
		case r == '<':
			depth++
		case r == '>':
			if depth > 0 {
				depth--
			}
		case depth == 0:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
