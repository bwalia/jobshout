package blog

import (
	"context"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/jobshoutcom"
	"github.com/jobshout/server/internal/model"
)

// InsightsPublisher is the slice of the JobShout.com client this package uses:
// filing an article in the Insights hub on jobshout.com. Declared here, where it
// is consumed, so tests can substitute a fake.
//
// It is a second destination beside the CMS, not a replacement. Both receive
// the same article; each tracks separately whether it has it.
type InsightsPublisher interface {
	SubmitInsight(ctx context.Context, req jobshoutcom.SubmitInsightRequest) (*jobshoutcom.Insight, error)
}

// WithInsights enables filing articles in JobShout.com Insights. Separate from
// NewRunner for the same reason as WithIllustrator: it is optional, and most
// callers build a Runner that does not need it.
func (r *Runner) WithInsights(p InsightsPublisher) *Runner {
	r.insights = p
	return r
}

// CanPublishInsights reports whether this Runner can reach Insights. As with
// CanPublish, main.go may pass a nil *jobshoutcom.Client inside a non-nil
// interface, so the dynamic value is checked too.
func (r *Runner) CanPublishInsights() bool {
	if r == nil || r.insights == nil {
		return false
	}
	if c, ok := r.insights.(*jobshoutcom.Client); ok {
		return c != nil
	}
	return true
}

// InsightsPost records where one article landed in Insights.
type InsightsPost struct {
	// Slug is JobShout's slug for the article, to match the result back.
	Slug string `json:"slug"`
	// ItemID and ItemSlug identify the Insights item; the slug is what the
	// jobshout.com URL is built from, and can differ from ours when Insights
	// had to de-duplicate it.
	ItemID   string `json:"item_id"`
	ItemSlug string `json:"item_slug"`
	Status   string `json:"status"`
}

// InsightsResult is returned once articles have been filed in Insights.
type InsightsResult struct {
	Posts       []InsightsPost `json:"posts"`
	PublishedAt time.Time      `json:"published_at"`
}

// PublishInsights files each article in the Insights review queue. Like the
// CMS path it never makes anything public: agent submissions always wait for
// an editor on jobshout.com.
//
// A failure part-way stops the batch and returns what was filed so far, so the
// caller can record those and a retry does not submit them twice.
func (r *Runner) PublishInsights(ctx context.Context, articles []GeneratedArticle, progress ProgressFunc) (*InsightsResult, error) {
	if !r.CanPublishInsights() {
		return nil, fmt.Errorf("blog: Insights is not configured (set JOBSHOUT_COM_API_URL and JOBSHOUT_INTERNAL_TOKEN)")
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("blog: nothing to publish")
	}

	result := &InsightsResult{Posts: make([]InsightsPost, 0, len(articles))}
	for i, a := range articles {
		report(progress, model.BlogStepInsightsSending,
			fmt.Sprintf("Filing %d/%d in Insights — %s", i+1, len(articles), a.Topic),
			model.AgentNameArticleWriter)

		item, err := r.insights.SubmitInsight(ctx, insightFromArticle(a, r.cfg.PublicBaseURL))
		if err != nil {
			result.PublishedAt = r.clock()
			return result, fmt.Errorf("blog: insights %d/%d: %w", i+1, len(articles), err)
		}
		result.Posts = append(result.Posts, InsightsPost{
			Slug: a.Slug, ItemID: item.ID, ItemSlug: item.Slug, Status: item.Status,
		})
	}

	result.PublishedAt = r.clock()
	r.logger.Info("blog: filed articles in Insights", zap.Int("articles", len(result.Posts)))
	report(progress, model.BlogStepInsightsSent,
		fmt.Sprintf("Filed %d article(s) for review in Insights", len(result.Posts)),
		model.AgentNameArticleWriter)
	return result, nil
}

// Limits the Insights API enforces; trimmed here so a long headline or
// summary files cleanly instead of failing validation.
const (
	insightsMaxTitle   = 160
	insightsMaxSummary = 300
)

// insightFromArticle builds the Insights submission for one article.
func insightFromArticle(a GeneratedArticle, publicBaseURL string) jobshoutcom.SubmitInsightRequest {
	title := strings.TrimSpace(a.Title)
	if title == "" {
		title = articleTitle(a.Markdown, a.Topic)
	}
	// The summary comes from the article as rendered now rather than the HTML
	// stored when it was written, so articles stored before a rendering fix
	// (a stray heading underline, say) still file with a clean summary.
	summary := strings.TrimSpace(a.Excerpt)
	if html, err := renderHTML(a.Markdown); err == nil && html != "" {
		summary = summaryParagraph(html)
	} else if summary == "" && a.HTML != "" {
		summary = articleExcerpt(a.HTML)
	}
	req := jobshoutcom.SubmitInsightRequest{
		Kind:         jobshoutcom.KindArticle,
		Title:        truncateRunes(title, insightsMaxTitle),
		Summary:      truncateRunes(summary, insightsMaxSummary),
		BodyMarkdown: insightsMarkdown(a.Markdown, publicBaseURL),
		Topics:       insightTopics(a.Topic + " " + title + " " + proseOnly(a.Markdown)),
		Submit:       true,
	}
	// Insights requires alt text with a cover; the cover is decorative art
	// drawn for the headline, so describing it by the headline is accurate.
	if cover := publicImageURL(publicBaseURL, a.CoverImageURL); cover != "" {
		req.CoverImageURL = cover
		req.CoverImageAlt = "Illustration for “" + truncateRunes(title, 120) + "”"
	}
	return req
}

// paragraphRegex matches one rendered paragraph.
var paragraphRegex = regexp.MustCompile(`(?s)<p>(.*?)</p>`)

// summaryParagraph is the opening prose of the article, for the Insights
// summary card. articleExcerpt (the CMS's) reads straight through headings,
// which runs a section title into the sentence after it; a summary reads
// better as the first real paragraph. Falls back to articleExcerpt when the
// body opens with nothing but images or headings.
func summaryParagraph(html string) string {
	for _, m := range paragraphRegex.FindAllStringSubmatch(html, -1) {
		if text := articleExcerpt(m[1]); text != "" {
			return text
		}
	}
	return articleExcerpt(html)
}

// relativeImage matches a Markdown image whose URL is a path on this host.
var relativeImage = regexp.MustCompile(`(!\[[^\]]*\]\()(/[^)\s]+)`)

// insightsMarkdown adapts the article's Markdown for Insights, which renders
// and sanitises Markdown itself:
//   - the leading H1 goes, because Insights shows the title separately;
//   - Mermaid fences become images from the public renderer, as on the CMS
//     path, since jobshout.com does not run mermaid.js;
//   - figures stored as /api/v1/images/… paths become absolute URLs on this
//     ring, or would resolve against jobshout.com and 404.
func insightsMarkdown(markdown, publicBaseURL string) string {
	md := stripLeadingH1(markdown)
	md = mermaidFence.ReplaceAllStringFunc(md, func(block string) string {
		m := mermaidFence.FindStringSubmatch(block)
		source := strings.TrimSpace(m[1])
		if source == "" {
			return block
		}
		return "![Diagram](" + mermaidInkSVG + base64.RawURLEncoding.EncodeToString([]byte(source)) + ")"
	})
	md = relativeImage.ReplaceAllStringFunc(md, func(img string) string {
		m := relativeImage.FindStringSubmatch(img)
		abs := publicImageURL(publicBaseURL, m[2])
		if abs == "" {
			return img
		}
		return m[1] + abs
	})
	return strings.TrimSpace(md)
}

// topicRules map an article onto the Insights topics (seeded by jobshout.com's
// migration). Order is priority: the first three that match win.
var topicRules = []struct {
	slug string
	re   *regexp.Regexp
}{
	{"job-market", regexp.MustCompile(`(?i)\b(job market|labou?r market|layoffs?|salar(y|ies)|unemployment|job postings?|demand for)\b`)},
	{"hiring", regexp.MustCompile(`(?i)\b(hiring|recruit\w*|interview\w*|candidates?|talent acquisition|onboarding)\b`)},
	{"skills-careers", regexp.MustCompile(`(?i)\b(skills?|careers?|upskill\w*|reskill\w*|learning path|certification)\b`)},
	{"policy-regulation", regexp.MustCompile(`(?i)\b(regulat\w*|polic(y|ies)|compliance|EU AI Act|legislation|governance|GDPR)\b`)},
	{"tools-research", regexp.MustCompile(`(?i)\b(benchmark\w*|research paper|open[- ]source|framework|library|SDK|tooling|kubernetes|API)\b`)},
	{"ai-trends", regexp.MustCompile(`(?i)\b(AI|LLMs?|machine learning|generative|agents?|models?)\b`)},
}

var (
	fencedCode = regexp.MustCompile("(?s)```.*?```")
	linkTarget = regexp.MustCompile(`\]\([^)]*\)`)
)

// proseOnly drops code blocks and link/image targets, so a URL such as
// /api/v1/images/… or a code sample cannot decide what an article is about.
func proseOnly(markdown string) string {
	return linkTarget.ReplaceAllString(fencedCode.ReplaceAllString(markdown, " "), "]")
}

// insightTopics picks up to three topics from the article text, always at
// least one: Insights will not accept a submission without a topic, and every
// article this agent writes is about AI in some way. An editor can re-file it.
func insightTopics(text string) []string {
	// Headings and the opening carry the subject; the long tail of the body
	// mentions everything once and would match every rule.
	if len(text) > 4000 {
		text = text[:4000]
	}
	var out []string
	for _, rule := range topicRules {
		if len(out) == 3 {
			break
		}
		if rule.re.MatchString(text) {
			out = append(out, rule.slug)
		}
	}
	if len(out) == 0 {
		out = []string{"ai-trends"}
	}
	return out
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)[:max-1]
	cut := string(r)
	if i := strings.LastIndexByte(cut, ' '); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " .,;:") + "…"
}
