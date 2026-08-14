package blog

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// GeneratedArticle is one produced article: the markdown the LLM wrote, the
// HTML it renders to, and the fields the CMS needs alongside the body.
type GeneratedArticle struct {
	Topic string
	Slug  string
	// Path is the markdown filename this article is filed under. Nothing writes
	// it to disk any more; it stays as the article's stable label in the UI.
	Path     string
	Markdown string
	// Title is the headline the agent chose from its research. It becomes the
	// CMS post title, and falls back to the article's H1 or the topic for
	// articles loaded back from before titles were stored.
	Title string
	// References are the verified sources this article cites, in citation
	// order. Populated by resolveCitations from the markers the writer used.
	References []model.BlogReference
	// HTML is Markdown rendered for the CMS, populated by render().
	HTML string
	// Excerpt is the post summary, derived from HTML — so it is empty until
	// render() has run.
	Excerpt string
	// WordCount is a rough size indicator shown in the UI so a reader can spot
	// a truncated or runaway article without opening it.
	WordCount int
}

// render fills HTML and Excerpt from Markdown. Separate from generation so the
// conversion is its own reportable step, and so an article loaded back from the
// database can be rendered without calling the LLM again.
func (a *GeneratedArticle) render() error {
	rendered, err := renderHTML(a.Markdown)
	if err != nil {
		return fmt.Errorf("blog: article %q: %w", a.Topic, err)
	}
	a.HTML = rendered
	a.Excerpt = articleExcerpt(rendered)
	if a.Title == "" {
		a.Title = articleTitle(a.Markdown, a.Topic)
	}
	return nil
}

// writeArticles produces one article per brief, in order.
//
// Any single brief failing aborts the batch — we prefer an all-or-nothing run
// over silently publishing half of what was asked for.
//
// progress fires at each phase of each article, so a long batch reports which
// article it is on and what it is doing rather than sitting on a single opaque
// "generating" step for several minutes.
func (r *Runner) writeArticles(
	ctx context.Context,
	req GenerateRequest,
	briefs []model.BlogBrief,
	progress ProgressFunc,
) ([]GeneratedArticle, error) {
	now := r.clock()
	out := make([]GeneratedArticle, 0, len(briefs))
	seenSlugs := map[string]int{}

	for i, brief := range briefs {
		label := fmt.Sprintf("%d/%d — %s", i+1, len(briefs), brief.Topic)

		article, err := r.writeOne(ctx, req, brief, label, progress)
		if err != nil {
			return nil, fmt.Errorf("blog: %q: %w", brief.Topic, err)
		}

		// Slugs come from the agent's title rather than the topic now: the
		// title is what the article is actually about, and it is what a reader
		// following the URL expects to find.
		slug := slugify(article.Title)
		base := slug
		// De-dupe within a single run so two articles that slugify to the same
		// value don't clobber each other.
		if n := seenSlugs[base]; n > 0 {
			slug = fmt.Sprintf("%s-%d", base, n+1)
		}
		seenSlugs[base]++

		filename := fmt.Sprintf("%s-%s.md", now.Format("2006-01-02"), slug)
		article.Slug = slug
		article.Path = strings.TrimRight(r.cfg.ContentDir, "/") + "/" + filename

		out = append(out, *article)
	}

	return out, nil
}

// writeOne runs the full agent loop for a single brief:
// research → plan → draft → review → revise → resolve citations.
//
// The sequence is fixed rather than being offered to the model as a set of
// choices. Research and citation resolution are guarantees this pipeline makes
// about its output, and a guarantee that the model can decide to skip is not
// one — the same reasoning that keeps the CMS contract out of the model's hands
// in this package's doc comment.
func (r *Runner) writeOne(
	ctx context.Context,
	req GenerateRequest,
	brief model.BlogBrief,
	label string,
	progress ProgressFunc,
) (*GeneratedArticle, error) {
	// 1. Research. Everything downstream is written from what this returns.
	report(progress, model.BlogStepResearching, "Researching "+label, model.AgentNameResearcher)
	rb, err := r.research.Research(ctx, req.OrgID, research.Request{
		Topic:   brief.Topic,
		Context: brief.Context,
		Model:   req.Model,
	}, func(_, detail string) {
		// The research agent's own phases are surfaced under the researching
		// step, so a reader watching a run sees it search and read rather than
		// watching one step sit still for a minute.
		report(progress, model.BlogStepResearching, detail, model.AgentNameResearcher)
	})
	if err != nil {
		return nil, fmt.Errorf("research: %w", err)
	}
	if !rb.IsUsable() {
		return nil, fmt.Errorf("research produced no verified findings to write from")
	}

	// 2. Plan — the title comes from what the research found.
	report(progress, model.BlogStepOutlining, "Planning "+label, model.AgentNameArticleWriter)
	plan, err := r.plan(ctx, req.Model, brief, rb)
	if err != nil {
		return nil, err
	}

	// 3. Draft.
	report(progress, model.BlogStepGenerating, "Writing "+plan.Title, model.AgentNameArticleWriter)
	markdown, err := r.draft(ctx, req.Model, brief, rb, plan)
	if err != nil {
		return nil, err
	}

	// 4. Review, then 5. revise — but only when there is something to fix.
	report(progress, model.BlogStepReviewing, "Reviewing "+plan.Title, model.AgentNameArticleWriter)
	c, err := r.review(ctx, req.Model, rb, plan, markdown)
	switch {
	case err != nil:
		// A failed review costs the revision pass, not the article. The draft
		// is already written from verified sources; discarding it because the
		// critic was unavailable would be a worse outcome than shipping it
		// unrevised.
		r.logger.Warn("blog: review failed, keeping the unrevised draft",
			zap.String("title", plan.Title), zap.Error(err))
	case len(c.Issues) == 0:
		r.logger.Info("blog: review found nothing to fix", zap.String("title", plan.Title))
	default:
		report(progress, model.BlogStepRevising,
			fmt.Sprintf("Revising %s (%d issue(s))", plan.Title, len(c.Issues)),
			model.AgentNameArticleWriter)
		revised, rerr := r.revise(ctx, req.Model, rb, plan, markdown, c)
		if rerr != nil {
			r.logger.Warn("blog: revision failed, keeping the reviewed draft",
				zap.String("title", plan.Title), zap.Error(rerr))
		} else {
			markdown = revised
		}
	}

	// 6. Expand if the piece came in short.
	//
	// Checked rather than trusted: the draft prompt asks for a word range and a
	// live run against a local model returned 382 words anyway. Revision can
	// also legitimately cut filler and take an already-brief article below the
	// floor, so the check belongs here, after revision, on whatever text
	// actually survived.
	if words := wordCount(markdown); words < MinArticleWords {
		report(progress, model.BlogStepExpanding,
			fmt.Sprintf("Expanding %s (%d words, target %d)", plan.Title, words, MinArticleWords),
			model.AgentNameArticleWriter)

		expanded, eerr := r.expand(ctx, req.Model, brief, rb, plan, markdown, words)
		switch {
		case eerr != nil:
			r.logger.Warn("blog: expansion failed, keeping the short article",
				zap.String("title", plan.Title), zap.Int("words", words), zap.Error(eerr))
		case wordCount(expanded) <= words:
			// A model that "expands" to something no longer than the original
			// has usually rewritten it shorter and blander. Keep what we had.
			r.logger.Warn("blog: expansion did not lengthen the article, keeping the original",
				zap.String("title", plan.Title),
				zap.Int("before", words), zap.Int("after", wordCount(expanded)))
		default:
			markdown = expanded
		}

		// One pass only. A second rarely adds substance, and each one costs a
		// full generation on a pipeline that already makes ten calls per
		// article. Still short is reported rather than retried into padding.
		if final := wordCount(markdown); final < MinArticleWords {
			r.logger.Warn("blog: article is below the target length",
				zap.String("title", plan.Title),
				zap.Int("words", final), zap.Int("target", MinArticleWords))
		}
	}

	// 7. Resolve citations into a reference list. This drops markers pointing
	// at sources that were never offered and renumbers what survives, so the
	// published article's references are exactly what it cites.
	rawCitations := countCitations(markdown)
	markdown, refs := resolveCitations(markdown, rb)
	markdown = strings.TrimRight(markdown, "\n") + referencesMarkdown(refs)

	if len(refs) == 0 {
		// Worth saying out loud: research verified sources and the writer then
		// cited none of them, so the article is unsourced despite the pipeline
		// having done its part.
		r.logger.Warn("blog: article cites no sources",
			zap.String("title", plan.Title),
			zap.Int("citation_markers_written", rawCitations),
			zap.Int("sources_available", len(rb.Findings)))
	}

	return &GeneratedArticle{
		Topic:      brief.Topic,
		Title:      plan.Title,
		Markdown:   markdown,
		References: refs,
		WordCount:  len(strings.Fields(markdown)),
	}, nil
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
