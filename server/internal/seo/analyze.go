package seo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// AnalyzeOptions controls a crawl of one URL.
type AnalyzeOptions struct {
	URL      string
	Keywords []string
	Timeout  time.Duration
}

// Analyze fetches the page (+ robots/sitemap) and builds a scored report.
func Analyze(ctx context.Context, opt AnalyzeOptions) (*ReportBundle, error) {
	raw := strings.TrimSpace(opt.URL)
	if raw == "" {
		return nil, fmt.Errorf("url is required")
	}
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("invalid url")
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 25 * time.Second
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 8 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "JobShout-SEO-Analyst/1.0 (+https://jobshout.co.uk)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch page: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	parsed := parseHTML(string(body))
	finalURL := resp.Request.URL.String()
	base, _ := url.Parse(finalURL)

	robotsURL := base.Scheme + "://" + base.Host + "/robots.txt"
	robotsBody, _ := fetchText(ctx, client, robotsURL)
	sitemapURL, sitemapFound, sitemapXML := discoverSitemap(ctx, client, base, robotsBody)

	cms := detectCMS(resp.Header, string(body), robotsBody)
	issues, suggestions := buildIssues(parsed, resp, opt.Keywords, sitemapFound, robotsBody)
	score := scoreFrom(parsed, resp, sitemapFound, robotsBody, issues)

	proposedSlug := slugify(parsed.Title)
	if proposedSlug == "" {
		proposedSlug = slugify(base.Path)
	}

	metaPatch := map[string]string{}
	if parsed.Title == "" || len(parsed.Title) < 15 || len(parsed.Title) > 60 {
		metaPatch["title"] = trimTo(preferTitle(parsed, opt.Keywords), 60)
	}
	if parsed.MetaDescription == "" || len(parsed.MetaDescription) < 50 || len(parsed.MetaDescription) > 160 {
		metaPatch["meta_description"] = trimTo(preferDescription(parsed, opt.Keywords), 155)
	}
	if len(parsed.H1) != 1 {
		metaPatch["h1"] = preferTitle(parsed, opt.Keywords)
	}
	if parsed.Canonical == "" {
		metaPatch["canonical"] = finalURL
	}

	genSitemap := ""
	if !sitemapFound {
		genSitemap = GenerateSitemapXML([]string{finalURL})
		suggestions = append(suggestions, Suggestion{
			ID: "add_sitemap", Kind: "sitemap",
			Title: "Add sitemap.xml",
			Detail: "No sitemap discovered. Generated a minimal sitemap for the homepage; expand with your page list before publish.",
			Proposed: "/sitemap.xml",
		})
	}

	report := &ReportBundle{
		FinalURL:        finalURL,
		StatusCode:      resp.StatusCode,
		Title:           parsed.Title,
		MetaDescription: parsed.MetaDescription,
		Canonical:       parsed.Canonical,
		H1:              parsed.H1,
		H2:              parsed.H2,
		KeywordsFound:   matchKeywords(parsed, opt.Keywords),
		RobotsTxt:       truncate(robotsBody, 2000),
		SitemapURL:      sitemapURL,
		SitemapFound:    sitemapFound,
		OpenGraph:       parsed.OpenGraph,
		Issues:          issues,
		Suggestions:     suggestions,
		ProposedSlug:    proposedSlug,
		SitemapXML:      firstNonEmpty(sitemapXML, genSitemap),
		MetaPatch:       metaPatch,
		DetectedCMS:     cms,
		Score:           score,
	}
	return report, nil
}

// ReportBundle is the analyzer output (maps onto model.SEOReport + SEOScore).
type ReportBundle struct {
	FinalURL        string
	StatusCode      int
	Title           string
	MetaDescription string
	Canonical       string
	H1, H2          []string
	KeywordsFound   []string
	RobotsTxt       string
	SitemapURL      string
	SitemapFound    bool
	OpenGraph       map[string]string
	Issues          []Issue
	Suggestions     []Suggestion
	ProposedSlug    string
	SitemapXML      string
	MetaPatch       map[string]string
	DetectedCMS     string
	Score           Score
}

// Issue / Suggestion / Score mirror model types without importing model here.
type Issue struct {
	ID, Severity, Category, Title, Detail, FixHint string
}
type Suggestion struct {
	ID, Kind, Title, Detail, Proposed string
}
type Score struct {
	Overall, IssueCount, CriticalCount int
	TitleOK, MetaDescOK, H1OK, CanonicalOK, RobotsOK, SitemapOK, OpenGraphOK, HTTPSOK bool
	ByCategory map[string]int
}

type pageBits struct {
	Title           string
	MetaDescription string
	Canonical       string
	H1, H2          []string
	OpenGraph       map[string]string
	Text            string
}

func parseHTML(src string) pageBits {
	doc, err := html.Parse(strings.NewReader(src))
	out := pageBits{OpenGraph: map[string]string{}}
	if err != nil {
		return out
	}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				out.Title = strings.TrimSpace(textContent(n))
			case "meta":
				name := attr(n, "name")
				prop := attr(n, "property")
				content := attr(n, "content")
				if strings.EqualFold(name, "description") {
					out.MetaDescription = strings.TrimSpace(content)
				}
				if strings.HasPrefix(strings.ToLower(prop), "og:") {
					out.OpenGraph[prop] = content
				}
			case "link":
				if strings.EqualFold(attr(n, "rel"), "canonical") {
					out.Canonical = strings.TrimSpace(attr(n, "href"))
				}
			case "h1":
				out.H1 = append(out.H1, strings.TrimSpace(textContent(n)))
			case "h2":
				if len(out.H2) < 12 {
					out.H2 = append(out.H2, strings.TrimSpace(textContent(n)))
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	out.Text = stripTags(src)
	return out
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

func textContent(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(x *html.Node) {
		if x.Type == html.TextNode {
			b.WriteString(x.Data)
		}
		for c := x.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

var tagRe = regexp.MustCompile(`(?s)<script.*?>.*?</script>|<style.*?>.*?</style>|<[^>]+>`)

func stripTags(s string) string {
	s = tagRe.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func fetchText(ctx context.Context, client *http.Client, raw string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "JobShout-SEO-Analyst/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func discoverSitemap(ctx context.Context, client *http.Client, base *url.URL, robots string) (sitemapURL string, found bool, xml string) {
	candidates := []string{}
	for _, line := range strings.Split(robots, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(strings.ToLower(line), "sitemap:") {
			candidates = append(candidates, strings.TrimSpace(line[8:]))
		}
	}
	candidates = append(candidates,
		base.Scheme+"://"+base.Host+"/sitemap.xml",
		base.Scheme+"://"+base.Host+"/sitemap_index.xml",
		base.Scheme+"://"+base.Host+"/wp-sitemap.xml",
	)
	seen := map[string]bool{}
	for _, c := range candidates {
		c = strings.TrimSpace(c)
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		body, err := fetchText(ctx, client, c)
		if err != nil || !strings.Contains(strings.ToLower(body), "<urlset") && !strings.Contains(strings.ToLower(body), "<sitemapindex") {
			continue
		}
		return c, true, body
	}
	return "", false, ""
}

func detectCMS(h http.Header, body, robots string) string {
	gen := strings.ToLower(h.Get("X-Generator") + h.Get("Link") + body[:min(len(body), 8000)])
	switch {
	case strings.Contains(gen, "wordpress") || strings.Contains(body, "wp-content") || strings.Contains(robots, "wp-"):
		return "wordpress"
	case strings.Contains(gen, "opsapi") || strings.Contains(body, "opsapi"):
		return "opsapi"
	case strings.Contains(gen, "next.js") || strings.Contains(body, "__NEXT_DATA__"):
		return "static"
	case strings.Contains(gen, "hugo") || strings.Contains(gen, "jekyll") || strings.Contains(gen, "gatsby"):
		return "static"
	default:
		return "unknown"
	}
}

func buildIssues(p pageBits, resp *http.Response, keywords []string, sitemapOK bool, robots string) ([]Issue, []Suggestion) {
	var issues []Issue
	var suggestions []Suggestion
	add := func(sev, cat, title, detail, fix string) {
		issues = append(issues, Issue{
			ID: fmt.Sprintf("%s_%d", cat, len(issues)+1),
			Severity: sev, Category: cat, Title: title, Detail: detail, FixHint: fix,
		})
	}
	if resp.StatusCode >= 400 {
		add("critical", "availability", "Page returned an error status",
			fmt.Sprintf("HTTP %d", resp.StatusCode), "Fix the URL or server response before SEO work.")
	}
	if resp.Request.URL.Scheme != "https" {
		add("high", "security", "Page is not served over HTTPS",
			"Search engines prefer HTTPS.", "Enable TLS and redirect HTTP → HTTPS.")
	}
	if strings.TrimSpace(p.Title) == "" {
		add("critical", "meta", "Missing <title>", "Every page needs a unique title.", "Set a 30–60 character title with the primary keyword.")
		suggestions = append(suggestions, Suggestion{ID: "title", Kind: "meta", Title: "Add page title", Detail: "Generate a concise title including focus keywords."})
	} else if len(p.Title) < 15 {
		add("medium", "meta", "Title is too short", fmt.Sprintf("%d chars", len(p.Title)), "Expand to 30–60 characters.")
	} else if len(p.Title) > 60 {
		add("medium", "meta", "Title may be truncated in SERPs", fmt.Sprintf("%d chars", len(p.Title)), "Shorten to ≤60 characters.")
	}
	if strings.TrimSpace(p.MetaDescription) == "" {
		add("high", "meta", "Missing meta description", "Improves click-through from search results.", "Write a 120–155 character description.")
		suggestions = append(suggestions, Suggestion{ID: "meta_desc", Kind: "meta", Title: "Add meta description", Detail: "Summarise the page value proposition."})
	} else if len(p.MetaDescription) < 50 || len(p.MetaDescription) > 160 {
		add("medium", "meta", "Meta description length is suboptimal",
			fmt.Sprintf("%d chars", len(p.MetaDescription)), "Aim for 120–155 characters.")
	}
	if len(p.H1) == 0 {
		add("high", "content", "Missing H1", "Pages should have exactly one H1.", "Add a single H1 matching the primary topic.")
	} else if len(p.H1) > 1 {
		add("medium", "content", "Multiple H1 headings", fmt.Sprintf("%d H1s found", len(p.H1)), "Keep a single H1; demote extras to H2.")
	}
	if p.Canonical == "" {
		add("medium", "meta", "Missing canonical link", "Helps avoid duplicate-content signals.", "Add <link rel=\"canonical\"> to the preferred URL.")
	}
	if robots == "" {
		add("low", "crawl", "robots.txt not found or empty", "Optional but useful for crawl control.", "Publish /robots.txt with a Sitemap: line.")
	}
	if !sitemapOK {
		add("high", "crawl", "Sitemap not found", "Search engines use sitemaps for discovery.", "Add /sitemap.xml and reference it from robots.txt.")
	}
	if len(p.OpenGraph) == 0 {
		add("low", "social", "No Open Graph tags", "Improves link previews.", "Add og:title, og:description, og:image.")
		suggestions = append(suggestions, Suggestion{ID: "og", Kind: "og", Title: "Add Open Graph tags", Detail: "Mirror title/description into og:* tags."})
	}
	for _, kw := range keywords {
		kw = strings.TrimSpace(strings.ToLower(kw))
		if kw == "" {
			continue
		}
		blob := strings.ToLower(p.Title + " " + p.MetaDescription + " " + strings.Join(p.H1, " ") + " " + p.Text)
		if !strings.Contains(blob, kw) {
			add("medium", "keywords", "Focus keyword not present",
				fmt.Sprintf("%q not found in title/meta/H1/body", kw),
				"Include the keyword naturally in title, H1, and early body copy.")
		}
	}
	return issues, suggestions
}

func scoreFrom(p pageBits, resp *http.Response, sitemapOK bool, robots string, issues []Issue) Score {
	s := Score{
		TitleOK: p.Title != "" && len(p.Title) >= 15 && len(p.Title) <= 60,
		MetaDescOK: p.MetaDescription != "" && len(p.MetaDescription) >= 50 && len(p.MetaDescription) <= 160,
		H1OK: len(p.H1) == 1,
		CanonicalOK: p.Canonical != "",
		RobotsOK: robots != "",
		SitemapOK: sitemapOK,
		OpenGraphOK: len(p.OpenGraph) > 0,
		HTTPSOK: resp.Request.URL.Scheme == "https",
		ByCategory: map[string]int{},
	}
	pts := 0
	checks := []bool{s.TitleOK, s.MetaDescOK, s.H1OK, s.CanonicalOK, s.RobotsOK, s.SitemapOK, s.OpenGraphOK, s.HTTPSOK, resp.StatusCode < 400}
	for _, ok := range checks {
		if ok {
			pts += 11
		}
	}
	if pts > 100 {
		pts = 100
	}
	s.Overall = pts
	s.IssueCount = len(issues)
	for _, i := range issues {
		s.ByCategory[i.Category]++
		if i.Severity == "critical" || i.Severity == "high" {
			s.CriticalCount++
		}
	}
	return s
}

func matchKeywords(p pageBits, kws []string) []string {
	blob := strings.ToLower(p.Title + " " + p.MetaDescription + " " + p.Text)
	var found []string
	for _, kw := range kws {
		kw = strings.TrimSpace(kw)
		if kw != "" && strings.Contains(blob, strings.ToLower(kw)) {
			found = append(found, kw)
		}
	}
	return found
}

func preferTitle(p pageBits, kws []string) string {
	if p.Title != "" {
		return p.Title
	}
	if len(p.H1) > 0 {
		return p.H1[0]
	}
	if len(kws) > 0 {
		return strings.Title(strings.TrimSpace(kws[0])) + " | Home"
	}
	return "Home"
}

func preferDescription(p pageBits, kws []string) string {
	if p.MetaDescription != "" {
		return p.MetaDescription
	}
	base := trimTo(p.Text, 140)
	if base == "" && len(kws) > 0 {
		return "Learn more about " + strings.Join(kws, ", ") + "."
	}
	return base
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonSlug.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 80 {
		s = s[:80]
		s = strings.Trim(s, "-")
	}
	return s
}

func trimTo(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return strings.TrimSpace(s[:n-1]) + "…"
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func firstNonEmpty(a, b string) string {
	if strings.TrimSpace(a) != "" {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// GenerateSitemapXML builds a minimal urlset for the given absolute URLs.
func GenerateSitemapXML(urls []string) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">` + "\n")
	now := time.Now().UTC().Format("2006-01-02")
	seen := map[string]bool{}
	for _, u := range urls {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		fmt.Fprintf(&b, "  <url><loc>%s</loc><lastmod>%s</lastmod><changefreq>weekly</changefreq><priority>0.8</priority></url>\n",
			xmlEscape(u), now)
	}
	b.WriteString("</urlset>\n")
	return b.String()
}

func xmlEscape(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;")
	return r.Replace(s)
}
