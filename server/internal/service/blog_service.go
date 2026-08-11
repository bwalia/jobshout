package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// BlogService orchestrates blog.Runner invocations and persists each run.
type BlogService interface {
	// Generate starts a run and returns immediately with the pending record.
	// The pipeline continues in the background; poll GetByID for progress.
	Generate(ctx context.Context, orgID uuid.UUID, triggeredBy *uuid.UUID, source string, req model.GenerateBlogRequest) (*model.BlogRun, error)
	// Publish pushes a completed run's articles and opens a pull request.
	Publish(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) (*model.BlogRun, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error)
	ListArticles(ctx context.Context, runID uuid.UUID) ([]model.BlogArticle, error)
	GetArticle(ctx context.Context, id uuid.UUID) (*model.BlogArticle, error)
	// CanPublish reports whether GitHub credentials are configured, so the UI
	// can disable the action instead of offering a button that always fails.
	CanPublish() bool
	// EnsureArticleWriter resolves the org's built-in Article Writer, creating
	// it if it is missing. Runs are attributed to it.
	EnsureArticleWriter(ctx context.Context, orgID uuid.UUID) (*model.Agent, error)
}

type blogService struct {
	runner    *blog.Runner
	repo      repository.BlogRepository
	agentRepo repository.AgentRepository
	logger    *zap.Logger
}

// NewBlogService creates a BlogService.
func NewBlogService(
	runner *blog.Runner,
	repo repository.BlogRepository,
	agentRepo repository.AgentRepository,
	logger *zap.Logger,
) BlogService {
	return &blogService{runner: runner, repo: repo, agentRepo: agentRepo, logger: logger}
}

func (s *blogService) CanPublish() bool {
	return s.runner != nil && s.runner.CanPublish()
}

// articleWriterSeed is the built-in agent definition. It must stay in step with
// the backfill in migration 000019 — that covers organizations which already
// existed, this covers everything created since.
func articleWriterSeed(orgID uuid.UUID) *model.Agent {
	desc := "Writes SEO-optimised technical articles in markdown, then publishes them to the content repository as a pull request for review."
	prompt := "You are a technical blog writer for a developer audience. You produce high-quality, SEO-optimised articles in pure markdown: a single H1 title, H2/H3 structure, 800-1200 words, at least one code block where it helps the reader, and a short Further Reading list."
	return &model.Agent{
		ID:           uuid.New(),
		OrgID:        orgID,
		Name:         "Article Writer",
		Role:         "Content Writer",
		Description:  &desc,
		SystemPrompt: &prompt,
		// 'active' rather than 'idle': the dashboard's Active Agents grid
		// filters on status = 'active', and this agent is always available.
		Status:       "active",
		EngineType:   model.EngineGoNative,
		EngineConfig: map[string]any{},
		Metadata:     map[string]any{model.MetadataKeyBuiltin: model.BuiltinArticleWriter},
	}
}

func (s *blogService) EnsureArticleWriter(ctx context.Context, orgID uuid.UUID) (*model.Agent, error) {
	existing, err := s.agentRepo.FindBuiltin(ctx, orgID, model.BuiltinArticleWriter)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	agent := articleWriterSeed(orgID)
	if err := s.agentRepo.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("blog_svc: seed article writer: %w", err)
	}
	s.logger.Info("blog: seeded built-in Article Writer agent",
		zap.String("org_id", orgID.String()), zap.String("agent_id", agent.ID.String()))
	return agent, nil
}

// initialSteps is the trace a run starts with. Steps are pre-seeded as pending
// so the UI can render the whole pipeline up front and light each one up as it
// happens, rather than growing a list from nothing.
func initialSteps() []model.BlogStep {
	return []model.BlogStep{
		{Key: model.BlogStepQueued, Label: "Queued", Status: model.StepStatusDone},
		{Key: model.BlogStepGenerating, Label: "Writing articles", Status: model.StepStatusPending},
		{Key: model.BlogStepGenerated, Label: "Articles ready", Status: model.StepStatusPending},
	}
}

// publishSteps are appended when a publish is requested, so a run that is only
// ever generated does not show phases it will never reach.
func publishSteps() []model.BlogStep {
	return []model.BlogStep{
		{Key: model.BlogStepPublishing, Label: "Committing to repository", Status: model.StepStatusPending},
		{Key: model.BlogStepOpeningPR, Label: "Opening pull request", Status: model.StepStatusPending},
		{Key: model.BlogStepPublished, Label: "Published", Status: model.StepStatusPending},
	}
}

// stepTracker owns a run's trace and persists it on every transition. Advancing
// to a step closes the previous one, which keeps "exactly one running step" true
// — the agent board's LATERAL lookup relies on that.
type stepTracker struct {
	runID  uuid.UUID
	steps  []model.BlogStep
	repo   repository.BlogRepository
	logger *zap.Logger
}

func (t *stepTracker) advance(key, label string) {
	now := time.Now()
	for i := range t.steps {
		if t.steps[i].Status == model.StepStatusRunning {
			t.steps[i].Status = model.StepStatusDone
			t.steps[i].CompletedAt = &now
		}
	}
	found := false
	for i := range t.steps {
		if t.steps[i].Key == key {
			t.steps[i].Status = model.StepStatusRunning
			// A step can be re-entered: "generating" fires once per topic,
			// relabelling itself each time. Keep the original start so the
			// duration covers the whole phase rather than the last topic, and
			// clear the completion the loop above just stamped — otherwise the
			// step reads as finished-in-0ms while it is still running.
			if t.steps[i].StartedAt == nil {
				t.steps[i].StartedAt = &now
			}
			t.steps[i].CompletedAt = nil
			if label != "" {
				t.steps[i].Label = label
			}
			found = true
			break
		}
	}
	if !found {
		t.steps = append(t.steps, model.BlogStep{
			Key: key, Label: label, Status: model.StepStatusRunning, StartedAt: &now,
		})
	}
	t.persist()
}

// finish closes the currently running step as done.
func (t *stepTracker) finish() {
	now := time.Now()
	for i := range t.steps {
		if t.steps[i].Status == model.StepStatusRunning {
			t.steps[i].Status = model.StepStatusDone
			t.steps[i].CompletedAt = &now
		}
	}
	t.persist()
}

// fail marks the running step failed and records why. Steps that never started
// stay pending, so the trace shows how far the run got.
func (t *stepTracker) fail(err error) {
	now := time.Now()
	msg := err.Error()
	for i := range t.steps {
		if t.steps[i].Status == model.StepStatusRunning {
			t.steps[i].Status = model.StepStatusFailed
			t.steps[i].CompletedAt = &now
			t.steps[i].Error = &msg
		}
	}
	t.persist()
}

func (t *stepTracker) persist() {
	// Best-effort: losing a progress write must never abort the run itself.
	if err := t.repo.UpdateSteps(context.Background(), t.runID, t.steps); err != nil {
		t.logger.Warn("blog: failed to persist step trace", zap.Error(err))
	}
}

func (s *blogService) Generate(
	ctx context.Context,
	orgID uuid.UUID,
	triggeredBy *uuid.UUID,
	source string,
	req model.GenerateBlogRequest,
) (*model.BlogRun, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("blog_svc: generator not configured")
	}

	agent, err := s.EnsureArticleWriter(ctx, orgID)
	if err != nil {
		return nil, err
	}

	startedAt := time.Now()
	run := &model.BlogRun{
		ID:          uuid.New(),
		OrgID:       orgID,
		AgentID:     &agent.ID,
		TriggeredBy: triggeredBy,
		Source:      source,
		Status:      model.BlogRunStatusRunning,
		Topics:      req.Topics,
		Steps:       initialSteps(),
		Articles:    []model.BlogRunArticle{},
		StartedAt:   &startedAt,
	}
	if req.Model != "" {
		m := req.Model
		run.Model = &m
	}

	if err := s.repo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("blog_svc: persist run: %w", err)
	}

	// Run in the background. Generation is 10-30s per topic on a warm host and
	// the request budget is 120s, so a full batch cannot fit in the response
	// — the caller polls the run instead.
	go s.runGeneration(run, agent, req)

	return run, nil
}

func (s *blogService) runGeneration(run *model.BlogRun, agent *model.Agent, req model.GenerateBlogRequest) {
	ctx := context.Background()
	log := s.logger.With(zap.String("blog_run_id", run.ID.String()))

	tracker := &stepTracker{runID: run.ID, steps: run.Steps, repo: s.repo, logger: s.logger}
	s.setAgentStatus(ctx, agent.ID, "active")

	articles, err := s.runner.Generate(ctx, blog.GenerateRequest{
		Topics:      req.Topics,
		Model:       req.Model,
		MaxArticles: req.MaxArticles,
	}, tracker.advance)

	completedAt := time.Now()
	run.CompletedAt = &completedAt

	if err != nil {
		tracker.fail(err)
		run.Steps = tracker.steps
		msg := err.Error()
		run.Status = model.BlogRunStatusFailed
		run.ErrorMessage = &msg
		if uerr := s.repo.Update(ctx, run); uerr != nil {
			log.Error("blog_svc: failed to record failure", zap.Error(uerr))
		}
		// Leave the agent active — 'failed' is a property of the run, and the
		// board reads the run, not the agent row.
		s.setAgentStatus(ctx, agent.ID, "active")
		log.Error("blog: generation failed", zap.Error(err))
		return
	}

	tracker.finish()
	run.Steps = tracker.steps

	persisted := make([]model.BlogArticle, 0, len(articles))
	summaries := make([]model.BlogRunArticle, 0, len(articles))
	for _, a := range articles {
		id := uuid.New()
		persisted = append(persisted, model.BlogArticle{
			ID: id, RunID: run.ID, OrgID: run.OrgID,
			Topic: a.Topic, Slug: a.Slug, Path: a.Path,
			Markdown: a.Markdown, WordCount: a.WordCount,
		})
		summaries = append(summaries, model.BlogRunArticle{
			ID: id, Topic: a.Topic, Slug: a.Slug, Path: a.Path, WordCount: a.WordCount,
		})
	}

	if err := s.repo.CreateArticles(ctx, persisted); err != nil {
		// The articles are the whole point of the run — if they cannot be
		// stored the run has not succeeded, however well generation went.
		msg := err.Error()
		run.Status = model.BlogRunStatusFailed
		run.ErrorMessage = &msg
		if uerr := s.repo.Update(ctx, run); uerr != nil {
			log.Error("blog_svc: failed to record failure", zap.Error(uerr))
		}
		log.Error("blog: storing articles failed", zap.Error(err))
		s.setAgentStatus(ctx, agent.ID, "active")
		return
	}

	run.Status = model.BlogRunStatusCompleted
	run.Articles = summaries
	if err := s.repo.Update(ctx, run); err != nil {
		log.Error("blog_svc: failed to persist success", zap.Error(err))
	}
	s.setAgentStatus(ctx, agent.ID, "active")
	log.Info("blog: generation complete", zap.Int("articles", len(articles)))
}

func (s *blogService) Publish(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) (*model.BlogRun, error) {
	if !s.CanPublish() {
		return nil, fmt.Errorf("blog_svc: publishing is not configured (GITHUB_TOKEN is unset)")
	}

	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, fmt.Errorf("blog_svc: run does not belong to this organization")
	}
	if run.Status != model.BlogRunStatusCompleted {
		return nil, fmt.Errorf("blog_svc: only a completed run can be published (status is %q)", run.Status)
	}
	if run.PRURL != nil {
		return nil, fmt.Errorf("blog_svc: run has already been published")
	}

	stored, err := s.repo.ListArticlesByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, fmt.Errorf("blog_svc: run has no articles to publish")
	}

	articles := make([]blog.GeneratedArticle, 0, len(stored))
	for _, a := range stored {
		articles = append(articles, blog.GeneratedArticle{
			Topic: a.Topic, Slug: a.Slug, Path: a.Path, Markdown: a.Markdown, WordCount: a.WordCount,
		})
	}

	run.Steps = append(run.Steps, publishSteps()...)
	tracker := &stepTracker{runID: run.ID, steps: run.Steps, repo: s.repo, logger: s.logger}

	result, err := s.runner.Publish(ctx, articles, tracker.advance)
	if err != nil {
		tracker.fail(err)
		run.Steps = tracker.steps
		msg := err.Error()
		run.ErrorMessage = &msg
		if uerr := s.repo.Update(ctx, run); uerr != nil {
			s.logger.Error("blog_svc: failed to record publish failure", zap.Error(uerr))
		}
		return nil, err
	}

	tracker.finish()
	run.Steps = tracker.steps
	run.Branch = &result.Branch
	pr := result.PRNumber
	run.PRNumber = &pr
	run.PRURL = &result.PRURL
	run.PublishedAt = &result.PublishedAt
	run.ErrorMessage = nil

	if err := s.repo.Update(ctx, run); err != nil {
		return nil, fmt.Errorf("blog_svc: persist publish: %w", err)
	}
	return run, nil
}

// setAgentStatus is best-effort: a status write failing must not fail the run.
func (s *blogService) setAgentStatus(ctx context.Context, agentID uuid.UUID, status string) {
	if err := s.agentRepo.UpdateStatus(ctx, agentID, status); err != nil {
		s.logger.Warn("blog: failed to update agent status", zap.Error(err))
	}
}

func (s *blogService) GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *blogService) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error) {
	return s.repo.ListByOrg(ctx, orgID, params)
}

func (s *blogService) ListArticles(ctx context.Context, runID uuid.UUID) ([]model.BlogArticle, error) {
	return s.repo.ListArticlesByRun(ctx, runID)
}

func (s *blogService) GetArticle(ctx context.Context, id uuid.UUID) (*model.BlogArticle, error) {
	return s.repo.GetArticle(ctx, id)
}
