package marketingemail

import (
	"html"
	"html/template"
	"net/url"
	"regexp"
	"strings"
)

// The email markdown allowlist: paragraphs, **bold**, *italic*, [links](https://…)
// and "- " / "1. " lists. Everything else is escaped text. There is no way to
// emit a tag, attribute, script, iframe, form or style from model text.

var (
	linkRe     = regexp.MustCompile(`\[([^\]\n]+)\]\(([^()\s]+)\)`)
	boldRe     = regexp.MustCompile(`\*\*([^*\n]+?)\*\*`)
	italicRe   = regexp.MustCompile(`\*([^*\s][^*\n]*?)\*`)
	bulletRe   = regexp.MustCompile(`^\s*[-*•]\s+(.*)$`)
	numberedRe = regexp.MustCompile(`^\s*\d{1,2}[.)]\s+(.*)$`)
	anyLinkRe  = regexp.MustCompile(`\]\(\s*([^)\s]*)\s*\)`)
)

// Inline styles are repeated on each tag: email clients drop <style> blocks.
const (
	pStyle  = `margin:0 0 16px 0;font-family:Arial,Helvetica,sans-serif;font-size:16px;line-height:1.6;color:#2b2a3a;`
	aStyle  = `color:#e5165f;text-decoration:underline;`
	ulStyle = `margin:0 0 16px 0;padding:0 0 0 22px;font-family:Arial,Helvetica,sans-serif;font-size:16px;line-height:1.6;color:#2b2a3a;`
	liStyle = `margin:0 0 6px 0;`
)

// nonHTTPSLinks returns the markdown link targets in s that are not https.
func nonHTTPSLinks(s string) []string {
	var bad []string
	for _, m := range anyLinkRe.FindAllStringSubmatch(s, -1) {
		if !IsHTTPS(m[1]) {
			bad = append(bad, m[1])
		}
	}
	return bad
}

// stripMarkers removes template markers so none can reach the output.
func stripMarkers(s string) string {
	s = strings.ReplaceAll(s, "{{", "{")
	s = strings.ReplaceAll(s, "}}", "}")
	return strings.ReplaceAll(s, "{%", "{")
}

// inlineHTML renders one line of inline markdown to safe HTML.
func inlineHTML(s string) string {
	s = stripMarkers(s)
	var out strings.Builder
	last := 0
	for _, loc := range linkRe.FindAllStringSubmatchIndex(s, -1) {
		out.WriteString(emphasis(s[last:loc[0]]))
		text, target := s[loc[2]:loc[3]], s[loc[4]:loc[5]]
		if IsHTTPS(target) && safeHref(target) {
			out.WriteString(`<a href="` + html.EscapeString(target) + `" style="` + aStyle + `" target="_blank" rel="noopener noreferrer">` + emphasis(text) + `</a>`)
		} else {
			out.WriteString(emphasis(text)) // a non-https link degrades to its text
		}
		last = loc[1]
	}
	out.WriteString(emphasis(s[last:]))
	return out.String()
}

// safeHref rejects targets with characters that have no business in a URL.
func safeHref(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.User == nil && !strings.ContainsAny(raw, "\"'<>` \t\r\n")
}

func emphasis(s string) string {
	s = html.EscapeString(s)
	s = boldRe.ReplaceAllString(s, `<strong>$1</strong>`)
	return italicRe.ReplaceAllString(s, `<em>$1</em>`)
}

// MarkdownHTML renders model markdown to the allowlisted HTML subset. The
// result is built here from escaped text, so it is safe as template.HTML.
func MarkdownHTML(md string) template.HTML {
	md = strings.ReplaceAll(strings.ReplaceAll(md, "\r\n", "\n"), "\r", "\n")
	var out strings.Builder
	var para []string
	var list []string
	ordered := false

	flushPara := func() {
		if len(para) == 0 {
			return
		}
		out.WriteString(`<p class="em-t" style="` + pStyle + `">` + inlineHTML(strings.Join(para, " ")) + `</p>`)
		para = nil
	}
	flushList := func() {
		if len(list) == 0 {
			return
		}
		tag := "ul"
		if ordered {
			tag = "ol"
		}
		out.WriteString(`<` + tag + ` class="em-t" style="` + ulStyle + `">`)
		for _, li := range list {
			out.WriteString(`<li class="em-t" style="` + liStyle + `">` + inlineHTML(li) + `</li>`)
		}
		out.WriteString(`</` + tag + `>`)
		list = nil
	}

	for _, line := range strings.Split(md, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flushPara()
			flushList()
			continue
		}
		if m := bulletRe.FindStringSubmatch(line); m != nil {
			flushPara()
			if len(list) > 0 && ordered {
				flushList()
			}
			ordered = false
			list = append(list, m[1])
			continue
		}
		if m := numberedRe.FindStringSubmatch(line); m != nil {
			flushPara()
			if len(list) > 0 && !ordered {
				flushList()
			}
			ordered = true
			list = append(list, m[1])
			continue
		}
		flushList()
		// Headings the model may emit anyway are plain text, not tags.
		para = append(para, strings.TrimLeft(trimmed, "# "))
	}
	flushPara()
	flushList()
	return template.HTML(out.String()) // #nosec G203 — built from escaped text and allowlisted tags
}
