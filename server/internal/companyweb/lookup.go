package companyweb

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"unicode"

	"github.com/jobshout/server/internal/research"
)

// Searcher is the web-search surface this agent needs (Brave via research.Client).
type Searcher interface {
	Available() bool
	Search(ctx context.Context, query string, limit int) ([]research.Source, error)
}

// Result is one ranked website candidate.
type Result struct {
	URL     string
	Title   string
	Site    string
	Score   int
	Excerpt string
}

// Lookup finds the best official website for companyName.
func Lookup(ctx context.Context, search Searcher, companyName string) ([]Result, error) {
	name := strings.TrimSpace(companyName)
	if name == "" {
		return nil, fmt.Errorf("company name is required")
	}
	if search == nil || !search.Available() {
		return nil, fmt.Errorf("web search is not configured on this server (needs Brave Search)")
	}

	queries := buildQueries(name)
	seen := map[string]Result{}
	for _, q := range queries {
		hits, err := search.Search(ctx, q, 8)
		if err != nil {
			return nil, err
		}
		for _, h := range hits {
			r := scoreHit(name, h)
			if r == nil {
				continue
			}
			key := strings.ToLower(r.Site)
			if prev, ok := seen[key]; !ok || r.Score > prev.Score {
				seen[key] = *r
			}
		}
	}

	out := make([]Result, 0, len(seen))
	for _, r := range seen {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return len(out[i].URL) < len(out[j].URL)
	})
	return out, nil
}

func buildQueries(name string) []string {
	return []string{
		fmt.Sprintf("%q official website", name),
		fmt.Sprintf("%s company website", name),
		fmt.Sprintf("%s official site", name),
	}
}

var skipHosts = map[string]bool{
	"linkedin.com": true, "www.linkedin.com": true,
	"facebook.com": true, "www.facebook.com": true, "fb.com": true,
	"twitter.com": true, "www.twitter.com": true, "x.com": true,
	"crunchbase.com": true, "www.crunchbase.com": true,
	"wikipedia.org": true, "en.wikipedia.org": true,
	"glassdoor.com": true, "www.glassdoor.com": true,
	"indeed.com": true, "www.indeed.com": true,
	"yelp.com": true, "www.yelp.com": true,
	"yellowpages.com": true, "www.yellowpages.com": true,
	"youtube.com": true, "www.youtube.com": true,
	"play.google.com": true, "apps.apple.com": true,
	"bloomberg.com": true, "www.bloomberg.com": true,
	"reuters.com": true, "www.reuters.com": true,
	"bbc.com": true, "www.bbc.com": true, "bbc.co.uk": true,
	"reddit.com": true, "www.reddit.com": true,
	"medium.com": true,
}

func scoreHit(companyName string, h research.Source) *Result {
	raw := strings.TrimSpace(h.URL)
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil
	}
	host := strings.ToLower(u.Hostname())
	if skipHosts[host] {
		return nil
	}
	// Skip other common directory / social TLDs by suffix.
	for _, bad := range []string{
		".linkedin.com", ".facebook.com", ".wikipedia.org",
		".crunchbase.com", ".glassdoor.com",
	} {
		if strings.HasSuffix(host, bad) {
			return nil
		}
	}

	site := h.Site
	if site == "" {
		site = strings.TrimPrefix(host, "www.")
	}
	site = strings.ToLower(site)

	score := 0
	nameTokens := tokens(companyName)
	hostTokens := tokens(strings.ReplaceAll(site, ".", " "))
	overlap := 0
	for _, t := range nameTokens {
		for _, htok := range hostTokens {
			if t == htok || strings.Contains(htok, t) || strings.Contains(t, htok) {
				overlap++
				break
			}
		}
	}
	score += overlap * 40
	titleLower := strings.ToLower(h.Title)
	for _, t := range nameTokens {
		if strings.Contains(titleLower, t) {
			score += 10
		}
	}

	path := u.Path
	if path == "" || path == "/" {
		score += 25
	} else if strings.EqualFold(path, "/en") || strings.EqualFold(path, "/home") ||
		strings.EqualFold(path, "/about") || strings.EqualFold(path, "/about-us") {
		score += 15
	} else if strings.Count(path, "/") >= 3 {
		score -= 15
	}

	if strings.EqualFold(u.Scheme, "https") {
		score += 5
	}
	if score < 20 {
		// Weak match — keep only if title clearly names the company.
		if overlap == 0 {
			return nil
		}
	}

	return &Result{
		URL:     raw,
		Title:   h.Title,
		Site:    site,
		Score:   score,
		Excerpt: h.Excerpt,
	}
}

func tokens(s string) []string {
	var b strings.Builder
	var out []string
	flush := func() {
		t := strings.ToLower(b.String())
		b.Reset()
		if t == "" {
			return
		}
		// Drop legal / filler suffixes that rarely appear in hostnames.
		switch t {
		case "ltd", "limited", "llc", "inc", "incorporated", "corp", "corporation",
			"plc", "gmbh", "co", "company", "the", "group", "holdings":
			return
		}
		if len(t) < 2 {
			return
		}
		out = append(out, t)
	}
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			flush()
		}
	}
	flush()
	return out
}
