package marketingemail

import (
	"regexp"
	"strings"

	"github.com/jobshout/server/internal/model"
)

var (
	mdLinkRe     = regexp.MustCompile(`\[([^\]]+)\]\((https://[^)\s]+)\)`)
	mdEmphasisRe = regexp.MustCompile(`\*{1,2}([^*]+)\*{1,2}`)
)

// plainInline turns one line of the markdown subset into plain text: links
// become "text (url)", emphasis markers go.
func plainInline(s string) string {
	s = stripMarkers(s)
	s = mdLinkRe.ReplaceAllString(s, "$1 ($2)")
	return strings.TrimSpace(mdEmphasisRe.ReplaceAllString(s, "$1"))
}

// RenderMailMergeText is the plain-text part of the mail-merge template, with
// the same merge fields as RenderMailMerge.
func RenderMailMergeText(d *model.MarketingEmailDraft, chrome Chrome) string {
	if d == nil {
		return ""
	}
	chrome = chrome.withDefaults()
	var sb strings.Builder
	line := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			sb.WriteString(s)
		}
		sb.WriteString("\n")
	}
	line("View this email in your browser: {{view_online_url}}")
	line("")
	line(plainInline(d.Hero.Headline))
	if d.Hero.Subline != "" {
		line(plainInline(d.Hero.Subline))
	}
	line("")
	line("Hi {{contact.first_name}},")
	for _, s := range d.Sections {
		line("")
		if s.Heading != "" {
			line(plainInline(s.Heading))
		}
		for _, l := range strings.Split(strings.ReplaceAll(s.BodyMarkdown, "\r\n", "\n"), "\n") {
			if t := strings.TrimSpace(l); t != "" {
				line(plainInline(t))
			}
		}
		for _, it := range s.Items {
			if it = plainInline(it); it != "" {
				line("- " + it)
			}
		}
		if u := strings.TrimSpace(s.CTAURL); s.CTALabel != "" && IsHTTPS(u) {
			line(plainInline(s.CTALabel) + ": " + u)
		}
	}
	line("")
	if d.FooterNote != "" {
		line(plainInline(d.FooterNote))
	}
	company := chrome.CompanyName
	if chrome.CompanyAddress != "" {
		company += " · " + chrome.CompanyAddress
	}
	line(company)
	line(chrome.HomeURL)
	line("Manage preferences: {{preferences_url}}")
	line("Unsubscribe: {{unsubscribe_url}}")
	return sb.String()
}
