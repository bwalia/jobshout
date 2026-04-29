// Package blog implements JobShout's automated blog-content pipeline:
// LLM generates markdown → git clone/commit/push → GitHub PR.
//
// The pipeline is a single deterministic function rather than a ReAct agent
// loop — git and the GitHub API are not operations where we want the LLM
// guessing at argument names.
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
	ContentDir  string // path within the repo where blogs are written
}

// GenerateRequest is the user-facing input.
type GenerateRequest struct {
	Topics      []string `json:"topics"`
	Model       string   `json:"model,omitempty"`        // optional override for the LLM
	MaxArticles int      `json:"max_articles,omitempty"` // safety cap; 0 = no cap below hard limit
}

// HardMaxArticles is the safety ceiling regardless of what the caller asks
// for. One PR with 25 blogs is almost certainly a mistake.
const HardMaxArticles = 10

// Result is returned to the caller.
type Result struct {
	Branch      string             `json:"branch"`
	PRNumber    int                `json:"pr_number"`
	PRURL       string             `json:"pr_url"`
	Articles    []GeneratedArticle `json:"articles"`
	GeneratedAt time.Time          `json:"generated_at"`
}

// Runner orchestrates the full generate-and-publish pipeline.
type Runner struct {
	cfg    Config
	llm    llm.Client
	pr     *github.PullRequestClient
	logger *zap.Logger
	// clock lets tests inject a deterministic time.
	clock func() time.Time
}

// NewRunner wires the Runner with its dependencies.
func NewRunner(cfg Config, llmClient llm.Client, logger *zap.Logger) *Runner {
	return &Runner{
		cfg:    cfg,
		llm:    llmClient,
		pr:     github.NewPullRequestClient(cfg.GitHubToken),
		logger: logger,
		clock:  time.Now,
	}
}

// Run executes the full pipeline. It is safe to call concurrently — each
// invocation gets its own clone directory keyed on the generated branch name.
func (r *Runner) Run(ctx context.Context, req GenerateRequest) (*Result, error) {
	if r.cfg.GitHubToken == "" {
		return nil, fmt.Errorf("blog: GITHUB_TOKEN is not set")
	}
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

	now := r.clock()
	branch := fmt.Sprintf("ai/blog-%s-%s", now.Format("2006-01-02"), randSuffix(now))

	// 1. Generate markdown for every topic before touching git. If the LLM
	//    fails we abort without creating a branch.
	articles, err := generateArticles(ctx, r.llm, req.Model, r.cfg.ContentDir, topics, now)
	if err != nil {
		return nil, err
	}

	// 2. Clone, branch, write files, commit, push.
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

	// 3. Open the PR.
	title := prTitle(articles)
	body := prBody(articles)
	createdPR, err := r.pr.CreatePullRequest(ctx,
		r.cfg.RepoOwner, r.cfg.RepoName, title, branch, r.cfg.BaseBranch, body)
	if err != nil {
		return nil, err
	}

	r.logger.Info("blog: run complete",
		zap.String("branch", branch),
		zap.Int("pr", createdPR.Number),
		zap.Int("articles", len(articles)),
	)

	return &Result{
		Branch:      branch,
		PRNumber:    createdPR.Number,
		PRURL:       createdPR.HTMLURL,
		Articles:    articles,
		GeneratedAt: now,
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
