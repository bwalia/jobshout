package blog

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/jobshout/server/internal/llm"
)

// BlogPromptTemplate is the system prompt given to the content LLM.
// Exported so tests can assert on it and callers can override it.
const BlogPromptTemplate = `You are a technical blog writer for a developer audience.

Generate a high-quality, SEO-optimised blog article on the topic below.

Topic:
%s

Requirements:
- Pure markdown output (no code fences around the whole response, no HTML).
- Start with a single H1 title line (# Title) derived from the topic.
- Use H2/H3 headings to structure the piece.
- 800–1200 words.
- Include at least one relevant code block where it helps the reader.
- End with a short "Further Reading" list.

Return only the markdown article — no preamble, no meta commentary.`

// GeneratedArticle is the produced markdown plus the path it should land at
// in the target repo.
type GeneratedArticle struct {
	Topic    string
	Slug     string
	Path     string // relative to repo root
	Markdown string
}

// generateArticles calls the LLM once per topic and returns the articles in
// the same order. Any single failure aborts the batch (we prefer atomic PRs
// over publishing half the batch silently).
func generateArticles(
	ctx context.Context,
	client llm.Client,
	model, contentDir string,
	topics []string,
	now time.Time,
) ([]GeneratedArticle, error) {
	out := make([]GeneratedArticle, 0, len(topics))
	seenSlugs := map[string]int{}

	for i, topic := range topics {
		topic = strings.TrimSpace(topic)
		if topic == "" {
			return nil, fmt.Errorf("blog: topic %d is empty", i)
		}

		resp, err := client.Generate(ctx, llm.GenerateRequest{
			Model: model,
			Messages: []llm.Message{
				{Role: "user", Content: fmt.Sprintf(BlogPromptTemplate, topic)},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("blog: generate topic %q: %w", topic, err)
		}

		md := strings.TrimSpace(resp.Content)
		if md == "" {
			return nil, fmt.Errorf("blog: generate topic %q: empty response", topic)
		}
		// Some models wrap the entire response in ```markdown … ``` — strip
		// that outer fence so the file is valid markdown.
		md = stripOuterFence(md)

		slug := slugify(topic)
		// De-dupe slugs within a single run so two topics that slugify to the
		// same value don't clobber each other.
		if n := seenSlugs[slug]; n > 0 {
			slug = fmt.Sprintf("%s-%d", slug, n+1)
		}
		seenSlugs[slugify(topic)]++

		filename := fmt.Sprintf("%s-%s.md", now.Format("2006-01-02"), slug)
		path := strings.TrimRight(contentDir, "/") + "/" + filename

		out = append(out, GeneratedArticle{
			Topic:    topic,
			Slug:     slug,
			Path:     path,
			Markdown: md,
		})
	}

	return out, nil
}

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

// slugify is the same URL-slug helper most blogs use: lowercase, ASCII-ish,
// hyphen-separated, bounded to 60 chars to avoid filesystem surprises.
func slugify(s string) string {
	s = strings.ToLower(s)
	s = slugRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 60 {
		s = s[:60]
		s = strings.TrimRight(s, "-")
	}
	if s == "" {
		return "untitled"
	}
	return s
}

// stripOuterFence removes a ```…``` (or ```lang…```) wrapper if the LLM
// wrapped the entire response in one.
func stripOuterFence(s string) string {
	trimmed := strings.TrimSpace(s)
	if !strings.HasPrefix(trimmed, "```") {
		return s
	}
	// Drop the opening fence line.
	nl := strings.Index(trimmed, "\n")
	if nl < 0 {
		return s
	}
	inner := trimmed[nl+1:]
	// Drop the closing fence.
	if idx := strings.LastIndex(inner, "```"); idx >= 0 {
		inner = inner[:idx]
	}
	return strings.TrimSpace(inner)
}
