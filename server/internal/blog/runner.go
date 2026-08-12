// Package blog implements JobShout's automated article pipeline:
// LLM generates markdown → markdown is rendered to HTML → the HTML is posted to
// the opsapi CMS as a draft.
//
// The pipeline is a set of deterministic functions rather than a ReAct agent
// loop — an HTTP contract is not somewhere we want the LLM guessing at field
// names. It is presented in the product as the built-in "Article Writer" agent;
// the agent identity is for visibility and attribution, the behaviour
// underneath stays fixed.
//
// Generation and publishing are deliberately separate. Generation needs no
// opsapi credentials and never leaves this system, so an article can be written
// and reviewed in the UI before anyone decides to send it. Publishing creates
// drafts, never live posts — the CMS is where a human approves the thing.
package blog

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// Config captures what the Runner needs. Populated from *config.Config in main.go.
type Config struct {
	// ContentDir is the directory the markdown filename is recorded under. It
	// no longer maps to a checkout — it survives as the article's stable,
	// human-readable identity in the UI and in blog_articles.path.
	ContentDir string
	// AuthorName is the byline attached to posts created in the CMS.
	AuthorName string
}

// CMSPublisher is the slice of the opsapi client this package uses. Declared
// here, where it is consumed, so tests can substitute a fake without reaching
// for the HTTP layer.
type CMSPublisher interface {
	CreatePost(ctx context.Context, req opsapi.CreatePostRequest) (*opsapi.Post, error)
	Namespace() string
}

// GenerateRequest is the user-facing input.
type GenerateRequest struct {
	Topics      []string `json:"topics"`
	Model       string   `json:"model,omitempty"`        // optional override for the LLM
	MaxArticles int      `json:"max_articles,omitempty"` // safety cap; 0 = no cap below hard limit
}

// HardMaxArticles is the safety ceiling regardless of what the caller asks
// for. One batch of 25 articles is almost certainly a mistake.
const HardMaxArticles = 10

// PostedArticle records where one article landed in the CMS.
type PostedArticle struct {
	// Slug is JobShout's slug for the article, used to match this result back
	// to the article it came from.
	Slug string `json:"slug"`
	// PostUUID is opsapi's identifier — what a user needs to open the draft.
	PostUUID string `json:"post_uuid"`
	// PostSlug is the slug opsapi settled on, which can differ from ours when
	// it had to de-duplicate within the namespace.
	PostSlug string `json:"post_slug"`
	Status   string `json:"status"`
}

// PublishResult is returned once articles have been posted to the CMS.
type PublishResult struct {
	Namespace   string          `json:"namespace"`
	Posts       []PostedArticle `json:"posts"`
	PublishedAt time.Time       `json:"published_at"`
}

// ProgressFunc is called as the pipeline moves between steps, so the caller can
// persist a live trace. It must not block for long — it runs inline.
//
// A nil ProgressFunc is valid; use report to stay nil-safe.
type ProgressFunc func(stepKey, label string)

func report(p ProgressFunc, stepKey, label string) {
	if p != nil {
		p(stepKey, label)
	}
}

// Runner orchestrates generation and publishing.
type Runner struct {
	cfg    Config
	llm    llm.Client
	cms    CMSPublisher
	logger *zap.Logger
	// clock lets tests inject a deterministic time.
	clock func() time.Time
}

// NewRunner wires the Runner with its dependencies. cms may be nil — generation
// still works, and only publishing is refused.
func NewRunner(cfg Config, llmClient llm.Client, cms CMSPublisher, logger *zap.Logger) *Runner {
	return &Runner{
		cfg:    cfg,
		llm:    llmClient,
		cms:    cms,
		logger: logger,
		clock:  time.Now,
	}
}

// CanPublish reports whether this Runner can reach the CMS. The API surfaces
// this so the UI can disable the Publish action rather than letting a user
// press a button that is guaranteed to fail.
//
// The nil check is on the interface's dynamic value as well: main.go passes a
// possibly-nil *opsapi.Client, which is a non-nil interface holding a nil
// pointer and would sail past a bare `r.cms != nil`.
func (r *Runner) CanPublish() bool {
	if r == nil || r.cms == nil {
		return false
	}
	if c, ok := r.cms.(*opsapi.Client); ok {
		return c != nil
	}
	return true
}

// Generate produces markdown for every requested topic, renders each to HTML,
// and returns both. It needs no CMS credentials.
//
// Any single topic failing aborts the batch — we prefer an all-or-nothing run
// over silently publishing half of what was asked for.
func (r *Runner) Generate(ctx context.Context, req GenerateRequest, progress ProgressFunc) ([]GeneratedArticle, error) {
	if r.llm == nil {
		return nil, fmt.Errorf("blog: llm client is nil")
	}

	topics := make([]string, 0, len(req.Topics))
	for _, t := range req.Topics {
		if s := strings.TrimSpace(t); s != "" {
			topics = append(topics, s)
		}
	}
	if len(topics) == 0 {
		return nil, fmt.Errorf("blog: at least one topic is required")
	}

	cap := req.MaxArticles
	if cap <= 0 || cap > HardMaxArticles {
		cap = HardMaxArticles
	}
	if len(topics) > cap {
		r.logger.Warn("blog: truncating topics to cap",
			zap.Int("requested", len(topics)),
			zap.Int("cap", cap),
		)
		topics = topics[:cap]
	}

	articles, err := generateArticles(ctx, r.llm, req.Model, r.cfg.ContentDir, topics, r.clock(), progress)
	if err != nil {
		return nil, err
	}

	// Rendering is its own step rather than part of generation: it is the point
	// where a malformed article stops being the LLM's problem and starts being
	// ours, and a reader watching the trace should see which one failed.
	report(progress, model.BlogStepConverting, fmt.Sprintf("Converting %d article(s) to HTML", len(articles)))
	for i := range articles {
		if err := articles[i].render(); err != nil {
			return nil, err
		}
	}

	report(progress, model.BlogStepGenerated, fmt.Sprintf("Generated %d article(s)", len(articles)))
	return articles, nil
}

// Publish creates one CMS draft per article.
//
// Posts go in as drafts without exception: this pipeline decides what gets
// written, not what a public site shows.
//
// A failure part-way leaves the earlier drafts in place. They are drafts, so
// nothing is visible to anyone, and deleting them to "clean up" would throw
// away work the user can simply publish again — the alternative, an
// all-or-nothing rollback, is not something the CMS API offers anyway.
func (r *Runner) Publish(ctx context.Context, articles []GeneratedArticle, progress ProgressFunc) (*PublishResult, error) {
	if !r.CanPublish() {
		return nil, fmt.Errorf("blog: publishing is not configured (set OPSAPI_BASE_URL, OPSAPI_TOKEN and OPSAPI_NAMESPACE)")
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("blog: nothing to publish")
	}

	namespace := r.cms.Namespace()
	posts := make([]PostedArticle, 0, len(articles))

	for i, a := range articles {
		report(progress, model.BlogStepPublishing,
			fmt.Sprintf("Posting %d/%d to %s — %s", i+1, len(articles), namespace, a.Topic))

		// Articles generated before HTML rendering existed have markdown but no
		// HTML. Render on the way out rather than refusing to publish them.
		if a.HTML == "" {
			if err := a.render(); err != nil {
				return nil, err
			}
		}

		post, err := r.cms.CreatePost(ctx, opsapi.CreatePostRequest{
			Title:       a.Title,
			Slug:        a.Slug,
			Excerpt:     a.Excerpt,
			ContentHTML: a.HTML,
			Status:      opsapi.StatusDraft,
			AuthorName:  r.cfg.AuthorName,
			SEOTitle:    a.Title,
			// opsapi caps meta descriptions at the same length we trim excerpts
			// to, so the excerpt serves both without a second derivation.
			SEODescription: a.Excerpt,
		})
		if err != nil {
			return nil, fmt.Errorf("blog: publish %d/%d: %w", i+1, len(articles), err)
		}

		posts = append(posts, PostedArticle{
			Slug:     a.Slug,
			PostUUID: post.UUID,
			PostSlug: post.Slug,
			Status:   post.Status,
		})
	}

	now := r.clock()
	r.logger.Info("blog: publish complete",
		zap.String("namespace", namespace),
		zap.Int("drafts", len(posts)),
	)

	report(progress, model.BlogStepPublished,
		fmt.Sprintf("Created %d draft(s) in %s", len(posts), namespace))

	return &PublishResult{
		Namespace:   namespace,
		Posts:       posts,
		PublishedAt: now,
	}, nil
}
