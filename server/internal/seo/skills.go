package seo

import (
	"strings"
)

// SkillHints documents how to locate site structure for common layouts.
// Used when a git repo / README / STRUCTURE.md is provided with the run.
var SkillHints = []string{
	"Look for README.md, STRUCTURE.md, or docs/seo.md describing content roots.",
	"Static sites: public/, dist/, docs/, or _site/ for generated HTML; content/ or src/pages/ for sources.",
	"WordPress: theme templates under wp-content/themes/*/header.php; XML sitemaps via Yoast/RankMath or /wp-sitemap.xml.",
	"OpsAPI: posts expose seo_title / seo_description; sitemap often at /sitemap.xml on the public host.",
	"Hugo: content/, layouts/, static/sitemap.xml or hugo config.",
	"Jekyll: _posts/, _layouts/, and optional sitemap plugin.",
	"Next.js: app/ or pages/; public/sitemap.xml; metadata in layout.tsx / next-seo.",
}

// InferSiteStructure guesses content/public roots from filenames (e.g. from a repo tree listing or md file).
func InferSiteStructure(paths []string, readme string) SiteStructure {
	s := SiteStructure{SkillNotes: append([]string{}, SkillHints...)}
	lowerReadme := strings.ToLower(readme)
	joined := strings.ToLower(strings.Join(paths, "\n"))

	pickRoot := func(candidates ...string) string {
		for _, c := range candidates {
			if strings.Contains(joined, "/"+c+"/") || strings.HasPrefix(joined, c+"/") || strings.Contains(joined, "\n"+c+"/") {
				return c
			}
		}
		return ""
	}

	s.ContentRoot = pickRoot("content", "src/pages", "pages", "app", "_posts", "posts")
	s.PublicRoot = pickRoot("public", "static", "dist", "docs", "_site", "out")
	switch {
	case strings.Contains(joined, "wp-content"):
		s.TemplateHint = "wordpress"
	case strings.Contains(joined, "hugo.toml") || strings.Contains(joined, "config.toml"):
		s.TemplateHint = "hugo"
	case strings.Contains(joined, "_config.yml"):
		s.TemplateHint = "jekyll"
	case strings.Contains(joined, "next.config"):
		s.TemplateHint = "nextjs"
	case strings.Contains(lowerReadme, "opsapi"):
		s.TemplateHint = "opsapi"
	default:
		s.TemplateHint = "static-html"
	}
	for _, p := range paths {
		lp := strings.ToLower(p)
		if strings.HasSuffix(lp, "sitemap.xml") || strings.HasSuffix(lp, "sitemap_index.xml") {
			s.HasSitemap = true
		}
		if strings.HasSuffix(lp, ".html") || strings.HasSuffix(lp, ".md") || strings.HasSuffix(lp, ".mdx") {
			if len(s.Pages) < 40 {
				s.Pages = append(s.Pages, p)
			}
		}
	}
	if strings.Contains(lowerReadme, "sitemap") {
		s.SkillNotes = append(s.SkillNotes, "README mentions sitemap — confirm path before regenerating.")
	}
	return s
}

// SiteStructure is the skill-inferred layout for static / git publish.
type SiteStructure struct {
	ContentRoot  string
	PublicRoot   string
	TemplateHint string
	Pages        []string
	HasSitemap   bool
	SkillNotes   []string
}

// ParseKeywords splits a comma/space keyword list.
func ParseKeywords(raw string) []string {
	raw = strings.ReplaceAll(raw, ";", ",")
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\t'
	})
	out := make([]string, 0, len(parts))
	seen := map[string]bool{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		k := strings.ToLower(p)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return out
}
