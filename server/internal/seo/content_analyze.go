package seo

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// AnalyzeContentOptions scores already-fetched HTML or markdown (GitHub files).
type AnalyzeContentOptions struct {
	Path     string // repo path or synthetic URL
	Content  string
	Keywords []string
	Kind     string // html | markdown | auto
}

// AnalyzeContent builds a ReportBundle from file contents (no network).
func AnalyzeContent(opt AnalyzeContentOptions) *ReportBundle {
	kind := strings.ToLower(strings.TrimSpace(opt.Kind))
	if kind == "" || kind == "auto" {
		lp := strings.ToLower(opt.Path)
		switch {
		case strings.HasSuffix(lp, ".md"), strings.HasSuffix(lp, ".mdx"), strings.HasSuffix(lp, ".markdown"):
			kind = "markdown"
		default:
			kind = "html"
		}
	}
	var parsed pageBits
	switch kind {
	case "markdown":
		parsed = parseMarkdown(opt.Content)
	default:
		parsed = parseHTML(opt.Content)
	}

	label := strings.TrimSpace(opt.Path)
	if label == "" {
		label = "page"
	}
	// Synthetic response for scoring helpers that expect HTTPS / 200.
	synth, _ := url.Parse("https://github.local/" + strings.TrimPrefix(label, "/"))
	resp := &http.Response{StatusCode: 200, Request: &http.Request{URL: synth}}

	issues, suggestions := buildIssues(parsed, resp, opt.Keywords, true, "User-agent: *\nAllow: /\n")
	// Drop crawl/security noise that does not apply to repo files.
	issues = filterFileIssues(issues)
	score := scoreFrom(parsed, resp, true, "ok", issues)
	score.RobotsOK = true
	score.SitemapOK = true
	score.HTTPSOK = true

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

	proposed := ""
	if kind == "markdown" {
		proposed = proposeFrontmatterPatch(parsed, metaPatch, opt.Keywords)
		if proposed != "" {
			suggestions = append(suggestions, Suggestion{
				ID: "frontmatter", Kind: "meta",
				Title: "Update frontmatter / headings",
				Detail: "Proposed YAML frontmatter and H1 aligned to focus keywords.",
				Proposed: proposed,
			})
		}
	} else if len(metaPatch) > 0 {
		suggestions = append(suggestions, Suggestion{
			ID: "html_meta", Kind: "meta",
			Title: "Update HTML meta tags",
			Detail: "Proposed title / meta description / H1 for this file.",
			Proposed: fmt.Sprintf("title: %s\nmeta_description: %s\nh1: %s",
				metaPatch["title"], metaPatch["meta_description"], metaPatch["h1"]),
		})
	}

	return &ReportBundle{
		FinalURL:        label,
		StatusCode:      200,
		Title:           parsed.Title,
		MetaDescription: parsed.MetaDescription,
		Canonical:       parsed.Canonical,
		H1:              parsed.H1,
		H2:              parsed.H2,
		KeywordsFound:   matchKeywords(parsed, opt.Keywords),
		SitemapFound:    true,
		OpenGraph:       parsed.OpenGraph,
		Issues:          issues,
		Suggestions:     suggestions,
		ProposedSlug:    slugify(parsed.Title),
		MetaPatch:       metaPatch,
		DetectedCMS:     "static",
		Score:           score,
	}
}

func filterFileIssues(in []Issue) []Issue {
	var out []Issue
	for _, i := range in {
		switch i.Category {
		case "availability", "security", "crawl":
			continue
		default:
			out = append(out, i)
		}
	}
	return out
}

var (
	fmRe   = regexp.MustCompile(`(?s)^---\s*\n(.*?)\n---\s*\n?`)
	h1MD   = regexp.MustCompile(`(?m)^#\s+(.+)$`)
	h2MD   = regexp.MustCompile(`(?m)^##\s+(.+)$`)
	titleFM = regexp.MustCompile(`(?mi)^title:\s*["']?(.+?)["']?\s*$`)
	descFM  = regexp.MustCompile(`(?mi)^(?:description|meta_description|seo_description|excerpt):\s*["']?(.+?)["']?\s*$`)
)

func parseMarkdown(src string) pageBits {
	out := pageBits{OpenGraph: map[string]string{}}
	body := src
	if m := fmRe.FindStringSubmatch(src); len(m) == 2 {
		fm := m[1]
		if tm := titleFM.FindStringSubmatch(fm); len(tm) == 2 {
			out.Title = strings.TrimSpace(tm[1])
		}
		if dm := descFM.FindStringSubmatch(fm); len(dm) == 2 {
			out.MetaDescription = strings.TrimSpace(dm[1])
		}
		body = src[len(m[0]):]
	}
	if hm := h1MD.FindStringSubmatch(body); len(hm) == 2 {
		out.H1 = append(out.H1, strings.TrimSpace(hm[1]))
		if out.Title == "" {
			out.Title = out.H1[0]
		}
	}
	for _, hm := range h2MD.FindAllStringSubmatch(body, 12) {
		if len(hm) == 2 {
			out.H2 = append(out.H2, strings.TrimSpace(hm[1]))
		}
	}
	// Strip markdown-ish markup for keyword body search.
	text := body
	text = regexp.MustCompile(`(?s)`+"```"+`.*?`+"```").ReplaceAllString(text, " ")
	text = regexp.MustCompile(`[\[\]\*_\x60#>-]`).ReplaceAllString(text, " ")
	out.Text = strings.Join(strings.Fields(text), " ")
	return out
}

func proposeFrontmatterPatch(p pageBits, patch map[string]string, kws []string) string {
	title := patch["title"]
	if title == "" {
		title = p.Title
	}
	desc := patch["meta_description"]
	if desc == "" {
		desc = p.MetaDescription
	}
	h1 := patch["h1"]
	if h1 == "" && len(p.H1) > 0 {
		h1 = p.H1[0]
	}
	if title == "" && desc == "" {
		return ""
	}
	var b strings.Builder
	b.WriteString("---\n")
	if title != "" {
		fmt.Fprintf(&b, "title: %q\n", title)
	}
	if desc != "" {
		fmt.Fprintf(&b, "description: %q\n", desc)
	}
	if len(kws) > 0 {
		fmt.Fprintf(&b, "keywords: [%s]\n", quoteJoin(kws))
	}
	b.WriteString("---\n\n")
	if h1 != "" {
		fmt.Fprintf(&b, "# %s\n", h1)
	}
	return b.String()
}

func quoteJoin(xs []string) string {
	parts := make([]string, 0, len(xs))
	for _, x := range xs {
		parts = append(parts, fmt.Sprintf("%q", x))
	}
	return strings.Join(parts, ", ")
}
