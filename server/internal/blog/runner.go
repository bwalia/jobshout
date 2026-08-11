// Package blog implements JobShout's automated article pipeline:
// LLM generates markdown → (optionally) git clone/commit/push → GitHub PR.
//
// The pipeline is a set of deterministic functions rather than a ReAct agent
// loop — git and the GitHub API are not operations where we want the LLM
// guessing at argument names. It is presented in the product as the built-in
// "Article Writer" agent; the agent identity is for visibility and attribution,
// the behaviour underneath stays fixed.
//
// Generation and publishing are deliberately separate. Generation needs no
// GitHub credentials and never touches a repository, so an article can be
// written and reviewed in the UI before anyone decides to ship it.
package blog

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration/adapters/github"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// Config captures what the Runner needs. Populated from *config.Config in main.go.
type Config struct {
	GitHubToken string
	AuthorName  string
	AuthorEmail string
	RepoOwner   string
	RepoName    string
	BaseBranch  string
	WorkDir     string
	ContentDir  string // path within the repo where articles are written
}

// GenerateRequest is the user-facing input.
type GenerateRequest struct {
	Topics      []string `json:"topics"`
	Model       string   `json:"model,omitempty"`        // optional override for the LLM
	MaxArticles int      `json:"max_articles,omitempty"` // safety cap; 0 = no cap below hard limit
}

// HardMaxArticles is the safety ceiling regardless of what the caller asks
// for. One PR with 25 articles is almost certainly a mistake.
const HardMaxArticles = 10

// PublishResult is returned once articles have been pushed and a PR opened.
type PublishResult struct {
	Branch      string    `json:"branch"`
	PRNumber    int       `json:"pr_number"`
	PRURL       string    `json:"pr_url"`
	PublishedAt time.Time `json:"published_at"`
}

// ProgressFunc is called as the pipeline moves between steps, so the caller can
// persist a live trace. It must not block for long — it runs inline.
//
// A nil ProgressFunc is valid; use progress() via reportTo to stay nil-safe.
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
	pr     *github.PullRequestClient
	logger *zap.Logger
	// clock lets tests inject a deterministic time.
	clock func() time.Time
}

// NewRunner wires the Runner with its dependencies. The GitHub token may be
// empty — generation still works, and only publishing is refused.
func NewRunner(cfg Config, llmClient llm.Client, logger *zap.Logger) *Runner {
	var prClient *github.PullRequestClient
	if cfg.GitHubToken != "" {
		prClient = github.NewPullRequestClient(cfg.GitHubToken)
	}
	return &Runner{
		cfg:    cfg,
		llm:    llmClient,
		pr:     prClient,
		logger: logger,
		clock:  time.Now,
	}
}

// CanPublish reports whether this Runner has the credentials to open a PR.
// The API surfaces this so the UI can disable the Publish action rather than
// letting a user press a button that is guaranteed to fail.
func (r *Runner) CanPublish() bool {
	return r.cfg.GitHubToken != "" && r.pr != nil
}

// Generate produces markdown for every requested topic. It never touches git
// and needs no GitHub credentials.
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

	report(progress, model.BlogStepGenerated, fmt.Sprintf("Generated %d article(s)", len(articles)))
	return articles, nil
}

// Publish clones the content repository, commits the given articles on a fresh
// branch, pushes, and opens a pull request.
func (r *Runner) Publish(ctx context.Context, articles []GeneratedArticle, progress ProgressFunc) (*PublishResult, error) {
	if !r.CanPublish() {
		return nil, fmt.Errorf("blog: publishing is not configured (GITHUB_TOKEN is unset)")
	}
	if len(articles) == 0 {
		return nil, fmt.Errorf("blog: nothing to publish")
	}

	now := r.clock()
	branch := fmt.Sprintf("ai/blog-%s-%s", now.Format("2006-01-02"), randSuffix(now))

	report(progress, model.BlogStepPublishing, "Committing to "+r.cfg.RepoOwner+"/"+r.cfg.RepoName)

	workDir := filepath.Join(r.cfg.WorkDir, branch)
	repo := newGitRepo(workDir, r.cfg.RepoOwner, r.cfg.RepoName, r.cfg.GitHubToken, r.cfg.AuthorName, r.cfg.AuthorEmail)
	defer repo.cleanup()

	if err := repo.clone(ctx, r.cfg.BaseBranch); err != nil {
		return nil, err
	}
	if err := repo.checkoutNewBranch(ctx, branch); err != nil {
		return nil, err
	}
	for _, a := range articles {
		if err := repo.writeFile(a.Path, a.Markdown); err != nil {
			return nil, err
		}
	}
	commitMsg := fmt.Sprintf("Add %d AI-generated blog post(s)\n\n%s",
		len(articles), topicBullets(articles))
	if err := repo.commit(ctx, commitMsg); err != nil {
		return nil, err
	}
	if err := repo.push(ctx, branch); err != nil {
		return nil, err
	}

	report(progress, model.BlogStepOpeningPR, "Opening pull request")

	createdPR, err := r.pr.CreatePullRequest(ctx,
		r.cfg.RepoOwner, r.cfg.RepoName, prTitle(articles), branch, r.cfg.BaseBranch, prBody(articles))
	if err != nil {
		return nil, err
	}

	r.logger.Info("blog: publish complete",
		zap.String("branch", branch),
		zap.Int("pr", createdPR.Number),
		zap.Int("articles", len(articles)),
	)

	report(progress, model.BlogStepPublished, fmt.Sprintf("Published as PR #%d", createdPR.Number))

	return &PublishResult{
		Branch:      branch,
		PRNumber:    createdPR.Number,
		PRURL:       createdPR.HTMLURL,
		PublishedAt: now,
	}, nil
}

// prTitle builds the PR title — single article gets a descriptive title,
// batches get a summary title.
func prTitle(articles []GeneratedArticle) string {
	if len(articles) == 1 {
		return "AI Generated Blog: " + articles[0].Topic
	}
	return fmt.Sprintf("AI Generated Blog: %d posts", len(articles))
}

// prBody is the PR description — includes all topics + file paths for review.
func prBody(articles []GeneratedArticle) string {
	var b strings.Builder
	b.WriteString("Auto-generated by JobShout. Review before merging.\n\n")
	b.WriteString("### Articles\n\n")
	for _, a := range articles {
		fmt.Fprintf(&b, "- **%s** — `%s`\n", a.Topic, a.Path)
	}
	return b.String()
}

func topicBullets(articles []GeneratedArticle) string {
	var out []string
	for _, a := range articles {
		out = append(out, "- "+a.Topic)
	}
	return strings.Join(out, "\n")
}

// randSuffix derives a short suffix from nanoseconds so two runs in the same
// second don't collide on branch name. Deterministic given the clock, so
// tests stay reproducible.
func randSuffix(t time.Time) string {
	n := t.UnixNano() % 0xFFFF
	return fmt.Sprintf("%04x", n)
}
