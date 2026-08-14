// Package blog implements JobShout's automated article pipeline:
// research → plan → draft → review → revise → render to HTML → post to the
// opsapi CMS as a draft.
//
// The writing itself is agentic: the Research Agent finds and verifies sources,
// the model chooses the article's title and structure from what that research
// found, and it then critiques and revises its own draft. What is *not* left to
// the model is the sequence. Research always happens, citations are always
// resolved against sources that were actually retrieved, and the CMS request is
// always built by this package — those are guarantees the pipeline makes about
// its output, and a guarantee a model can decide to skip is not one. So the
// model decides what to say and the pipeline decides what must be true of it.
//
// It is presented in the product as the built-in "Article Writer" agent, and
// commissions the built-in "Research Agent" for the sources it writes from.
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

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/opsapi"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
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
	// OrgID scopes the research the pipeline commissions, so a run is
	// attributed to the right organization's Research Agent.
	OrgID       uuid.UUID
	Briefs      []model.BlogBrief `json:"briefs"`
	Model       string            `json:"model,omitempty"`        // optional override for the LLM
	MaxArticles int               `json:"max_articles,omitempty"` // safety cap; 0 = no cap below hard limit
}

// Researcher is the slice of service.ResearchService this package consumes.
// Declared here, where it is used, so blog does not import service — and so a
// test can supply a brief without a network or an LLM.
type Researcher interface {
	Research(ctx context.Context, orgID uuid.UUID, req research.Request, progress research.ProgressFunc) (*research.Brief, error)
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
// agent names which agent is doing the work, so the trace shows the handover
// between the Research Agent and the Article Writer rather than presenting a
// run as one anonymous process.
//
// A nil ProgressFunc is valid; use report to stay nil-safe.
type ProgressFunc func(stepKey, label, agent string)

func report(p ProgressFunc, stepKey, label, agent string) {
	if p != nil {
		p(stepKey, label, agent)
	}
}

// Runner orchestrates generation and publishing.
type Runner struct {
	cfg      Config
	llm      llm.Client
	cms      CMSPublisher
	research Researcher
	logger   *zap.Logger
	// clock lets tests inject a deterministic time.
	clock func() time.Time
}

// NewRunner wires the Runner with its dependencies. cms may be nil — generation
// still works, and only publishing is refused.
//
// researcher may not be nil. Every article this pipeline produces is written
// from verified sources, so a Runner with nowhere to get them cannot do its
// job — and failing at construction is better than discovering it per-article.
func NewRunner(cfg Config, llmClient llm.Client, cms CMSPublisher, researcher Researcher, logger *zap.Logger) *Runner {
	return &Runner{
		cfg:      cfg,
		llm:      llmClient,
		cms:      cms,
		research: researcher,
		logger:   logger,
		clock:    time.Now,
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

// CMSNamespace is the namespace publishing targets, or "" when the CMS is not
// configured. Callers use it to record where a run's drafts went.
func (r *Runner) CMSNamespace() string {
	if !r.CanPublish() {
		return ""
	}
	return r.cms.Namespace()
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
	if r.research == nil {
		return nil, fmt.Errorf("blog: research is not configured — articles are written from verified sources and there are none available")
	}

	briefs := make([]model.BlogBrief, 0, len(req.Briefs))
	for _, b := range req.Briefs {
		if s := strings.TrimSpace(b.Topic); s != "" {
			briefs = append(briefs, model.BlogBrief{Topic: s, Context: strings.TrimSpace(b.Context)})
		}
	}
	if len(briefs) == 0 {
		return nil, fmt.Errorf("blog: at least one topic is required")
	}

	cap := req.MaxArticles
	if cap <= 0 || cap > HardMaxArticles {
		cap = HardMaxArticles
	}
	if len(briefs) > cap {
		r.logger.Warn("blog: truncating briefs to cap",
			zap.Int("requested", len(briefs)),
			zap.Int("cap", cap),
		)
		briefs = briefs[:cap]
	}

	articles, err := r.writeArticles(ctx, req, briefs, progress)
	if err != nil {
		return nil, err
	}

	// Rendering is its own step rather than part of generation: it is the point
	// where a malformed article stops being the LLM's problem and starts being
	// ours, and a reader watching the trace should see which one failed.
	report(progress, model.BlogStepConverting, fmt.Sprintf("Converting %d article(s) to HTML", len(articles)), model.AgentNameArticleWriter)
	for i := range articles {
		if err := articles[i].render(); err != nil {
			return nil, err
		}
	}

	report(progress, model.BlogStepGenerated, fmt.Sprintf("Generated %d article(s)", len(articles)), model.AgentNameArticleWriter)
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
			fmt.Sprintf("Posting %d/%d to %s — %s", i+1, len(articles), namespace, a.Topic),
			model.AgentNameArticleWriter)

		// Title and Excerpt are derived, not stored: an article loaded back from
		// blog_articles arrives with markdown and HTML and nothing else. Fill
		// them in here, or opsapi rejects the post outright for having no title.
		//
		// The stored HTML is kept as-is when present — it is what was reviewed
		// in the UI, so it is what should be sent. Only articles written before
		// the conversion step existed are rendered on the way out.
		if a.HTML == "" {
			if err := a.render(); err != nil {
				return nil, err
			}
		} else {
			if a.Title == "" {
				a.Title = articleTitle(a.Markdown, a.Topic)
			}
			if a.Excerpt == "" {
				a.Excerpt = articleExcerpt(a.HTML)
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
		fmt.Sprintf("Created %d draft(s) in %s", len(posts), namespace),
		model.AgentNameArticleWriter)

	return &PublishResult{
		Namespace:   namespace,
		Posts:       posts,
		PublishedAt: now,
	}, nil
}
