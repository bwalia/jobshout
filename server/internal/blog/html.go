package blog

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
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
//
// Hard wraps are deliberately NOT enabled. A model that soft-wraps a paragraph
// at ~80 columns is expressing nothing by it, and turning those newlines into
// <br> puts forced breaks in the middle of running prose. Without the option a
// wrapped paragraph reflows correctly, and a model that wanted a real break can
// still get one the CommonMark way.
var markdownConverter = goldmark.New(
	goldmark.WithExtensions(extension.GFM),
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
	return strings.TrimSpace(mermaidToImg(buf.String())), nil
}

// mermaidCodeBlock matches the HTML goldmark emits for a ```mermaid fence.
var mermaidCodeBlock = regexp.MustCompile(
	`(?s)<pre><code class="language-mermaid">(.*?)</code></pre>`)

// mermaidInkSVG is the public Mermaid renderer used for CMS HTML. The opsapi
// TipTap editor and consumer sites that inject content_html do not run
// mermaid.js, so a bare <div class="mermaid"> never becomes a diagram there.
// An <img> pointing at mermaid.ink does: TipTap keeps images, and a plain
// HTML page paints them without any client-side Mermaid runtime.
const mermaidInkSVG = "https://mermaid.ink/svg/"

// mermaidToImg rewrites a fenced mermaid block into a self-rendering <img>.
//
// goldmark has no idea what mermaid is, so it renders the fence as a code
// block — which would publish the diagram's source as literal text. The
// previous <div class="mermaid"> convention only helps a theme that loads
// mermaid.js; the CMS path JobShout publishes to does not.
//
// The diagram source is URL-safe base64 in the image path (not HTML body
// text), so characters like < and > cannot become markup.
func mermaidToImg(html string) string {
	return mermaidCodeBlock.ReplaceAllStringFunc(html, func(block string) string {
		m := mermaidCodeBlock.FindStringSubmatch(block)
		if m == nil {
			return block
		}
		source := strings.TrimSpace(unescapeEntities(m[1]))
		if source == "" {
			return block
		}
		enc := base64.RawURLEncoding.EncodeToString([]byte(source))
		return `<img class="mermaid-diagram" alt="Diagram" src="` + mermaidInkSVG + enc + `" />`
	})
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
