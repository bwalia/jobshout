package blog

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer/html"
)

// markdownConverter renders the markdown the LLM produces into the HTML the
// CMS stores.
//
// GFM is enabled because models reach for tables and strikethrough whether or
// not the prompt asks for them, and CommonMark alone would emit those as
// literal pipes and tildes.
//
// Raw HTML is *not* unsafe-rendered: the prompt asks for pure markdown, so any
// HTML in the output is the model going off-script rather than intent, and the
// result is written straight into a page on a public site. Goldmark's default
// escapes it, which is what we want.
var markdownConverter = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
	goldmark.WithRendererOptions(
		// Preserve hard line breaks inside paragraphs; models use them for
		// list-like passages that are not actually lists.
		html.WithHardWraps(),
	),
)

// renderHTML converts an article's markdown to HTML for the CMS.
//
// The leading H1 is dropped: the CMS renders the post title from its own title
// field, so keeping the markdown's H1 would show it twice on the page.
func renderHTML(markdown string) (string, error) {
	var buf bytes.Buffer
	if err := markdownConverter.Convert([]byte(stripLeadingH1(markdown)), &buf); err != nil {
		return "", fmt.Errorf("blog: render markdown to html: %w", err)
	}
	return strings.TrimSpace(buf.String()), nil
}

var (
	// h1Regex matches the first ATX H1 line, which generateArticles asks the
	// model to open with.
	//
	// The character classes exclude newlines deliberately: \s would let a bare
	// "#" line match across the blank line after it and adopt the first
	// paragraph as the article's title.
	h1Regex = regexp.MustCompile(`(?m)^#[^\S\n]+([^\n]+?)[^\S\n]*$`)
	// htmlTagRegex strips tags when deriving plain-text fields from rendered
	// HTML, so an excerpt never carries markup into a meta description.
	htmlTagRegex = regexp.MustCompile(`<[^>]*>`)
	// whitespaceRegex collapses the newlines left behind by tag removal.
	whitespaceRegex = regexp.MustCompile(`\s+`)
)

// articleTitle is the post title: the markdown's H1 when the model followed the
// prompt, otherwise the topic it was asked to write about.
func articleTitle(markdown, topic string) string {
	if m := h1Regex.FindStringSubmatch(markdown); m != nil {
		if title := strings.TrimSpace(m[1]); title != "" {
			return title
		}
	}
	return topic
}

// stripLeadingH1 removes the article's own H1 heading, and only that one —
// an H1 used later in the body (rare, but models do it) stays put.
func stripLeadingH1(markdown string) string {
	trimmed := strings.TrimLeft(markdown, " \t\r\n")
	if !strings.HasPrefix(trimmed, "# ") {
		return markdown
	}
	if nl := strings.IndexByte(trimmed, '\n'); nl >= 0 {
		return strings.TrimLeft(trimmed[nl+1:], "\r\n")
	}
	// The whole article is a single H1 line — nothing left once it goes.
	return ""
}

// excerptLimit is the length an excerpt is trimmed to. It also feeds the SEO
// description, where search engines stop showing text at around 160 characters.
const excerptLimit = 160

// articleExcerpt derives the post's summary from the rendered HTML — the first
// stretch of prose, with markup and headings already resolved.
//
// Trimming happens on a word boundary so the excerpt does not end mid-word.
func articleExcerpt(htmlBody string) string {
	text := htmlTagRegex.ReplaceAllString(htmlBody, " ")
	text = strings.TrimSpace(whitespaceRegex.ReplaceAllString(unescapeEntities(text), " "))
	if text == "" {
		return ""
	}
	if len(text) <= excerptLimit {
		return text
	}
	cut := text[:excerptLimit]
	if idx := strings.LastIndexByte(cut, ' '); idx > 0 {
		cut = cut[:idx]
	}
	return strings.TrimRight(cut, " .,;:") + "…"
}

// unescapeEntities reverses the handful of entities goldmark emits, so an
// excerpt taken from HTML reads as plain text rather than showing &amp;.
func unescapeEntities(s string) string {
	return strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
	).Replace(s)
}
