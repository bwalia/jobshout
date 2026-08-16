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
	"github.com/jobshout/server/internal/research"
)

// BlogService orchestrates blog.Runner invocations and persists each run.
type BlogService interface {
	// Generate starts a run and returns immediately with the pending record.
	// The pipeline continues in the background; poll GetByID for progress.
	Generate(ctx context.Context, orgID uuid.UUID, triggeredBy *uuid.UUID, source string, req model.GenerateBlogRequest) (*model.BlogRun, error)
	// Publish creates one CMS draft per article of a completed run.
	Publish(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) (*model.BlogRun, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error)
	// Delete removes a run and everything it produced.
	Delete(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) error
	// Retry re-runs a failed run's original topics in place, so a transient
	// failure does not leave a dead card the user has to clean up by hand.
	Retry(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) (*model.BlogRun, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error)
	ListArticles(ctx context.Context, runID uuid.UUID) ([]model.BlogArticle, error)
	GetArticle(ctx context.Context, id uuid.UUID) (*model.BlogArticle, error)
	// CanPublish reports whether the CMS connection is configured, so the UI
	// can disable the action instead of offering a button that always fails.
	CanPublish() bool
	// EnsureArticleWriter resolves the org's built-in Article Writer, creating
	// it if it is missing. Runs are attributed to it.
	EnsureArticleWriter(ctx context.Context, orgID uuid.UUID) (*model.Agent, error)
}

type blogService struct {
	runner *blog.Runner
	repo   repository.BlogRepository
	// research is used only for topic discovery. Article research goes through
	// the runner, which holds its own reference — this one is for runs that
	// arrive without a subject at all.
	research  ResearchService
	agentRepo repository.AgentRepository
	logger    *zap.Logger
}

// NewBlogService creates a BlogService.
func NewBlogService(
	runner *blog.Runner,
	repo repository.BlogRepository,
	research ResearchService,
	agentRepo repository.AgentRepository,
	logger *zap.Logger,
) BlogService {
	return &blogService{
		runner:    runner,
		repo:      repo,
		research:  research,
		agentRepo: agentRepo,
		logger:    logger,
	}
}

func (s *blogService) CanPublish() bool {
	return s.runner != nil && s.runner.CanPublish()
}

// articleWriterSeed is the built-in agent definition. It must stay in step with
// the backfill in migration 000019 — that covers organizations which already
// existed, this covers everything created since.
func articleWriterSeed(orgID uuid.UUID) *model.Agent {
	desc := "Writes SEO-optimised technical articles in markdown, converts them to HTML, and files them in the CMS as drafts for review."
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
// The agent on each step is seeded here rather than only stamped when the step
// runs, so the trace shows who will do what before anything has started.
//
// discovering adds the topic-discovery step, which only exists on runs that
// were not given a subject. Seeding it conditionally keeps a normal run's trace
// free of a step it will never reach.
func initialSteps(discovering bool) []model.BlogStep {
	writer, researcher := model.AgentNameArticleWriter, model.AgentNameResearcher
	steps := []model.BlogStep{
		{Key: model.BlogStepQueued, Label: "Queued", Status: model.StepStatusDone},
	}
	if discovering {
		steps = append(steps, model.BlogStep{
			Key:    model.BlogStepDiscovering,
			Label:  "Finding what is trending",
			Agent:  researcher,
			Status: model.StepStatusPending,
		})
	}
	return append(steps, writeSteps(writer, researcher)...)
}

// writeSteps is the per-article pipeline, shared by every run.
func writeSteps(writer, researcher string) []model.BlogStep {
	return []model.BlogStep{
		{Key: model.BlogStepResearching, Label: "Researching sources", Agent: researcher, Status: model.StepStatusPending},
		{Key: model.BlogStepOutlining, Label: "Choosing a title and outline", Agent: writer, Status: model.StepStatusPending},
		{Key: model.BlogStepGenerating, Label: "Writing articles", Agent: writer, Status: model.StepStatusPending},
		{Key: model.BlogStepReviewing, Label: "Reviewing the draft", Agent: writer, Status: model.StepStatusPending},
		{Key: model.BlogStepRevising, Label: "Revising", Agent: writer, Status: model.StepStatusPending},
		{Key: model.BlogStepExpanding, Label: "Expanding to full length", Agent: writer, Status: model.StepStatusPending},
		{Key: model.BlogStepConverting, Label: "Converting to HTML", Agent: writer, Status: model.StepStatusPending},
		{Key: model.BlogStepGenerated, Label: "Articles ready", Agent: writer, Status: model.StepStatusPending},
	}
}

// publishSteps are appended when a publish is requested, so a run that is only
// ever generated does not show phases it will never reach.
func publishSteps() []model.BlogStep {
	return []model.BlogStep{
		{Key: model.BlogStepPublishing, Label: "Posting drafts to the CMS", Status: model.StepStatusPending},
		{Key: model.BlogStepPublished, Label: "Drafts created", Status: model.StepStatusPending},
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

func (t *stepTracker) advance(key, label, agent string) {
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
			if agent != "" {
				t.steps[i].Agent = agent
			}
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
			Key: key, Label: label, Agent: agent, Status: model.StepStatusRunning, StartedAt: &now,
		})
	}
	t.persist()
}

// finish closes the currently running step as done, and marks anything that
// never started as skipped.
//
// The skip pass matters because the trace is pre-seeded with the whole
// pipeline: a run where the reviewer found nothing to fix never enters the
// revising step, and leaving it pending on a completed run makes finished work
// look stalled.
func (t *stepTracker) finish() {
	now := time.Now()
	for i := range t.steps {
		switch t.steps[i].Status {
		case model.StepStatusRunning:
			t.steps[i].Status = model.StepStatusDone
			t.steps[i].CompletedAt = &now
		case model.StepStatusPending:
			t.steps[i].Status = model.StepStatusSkipped
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

	// Normalize is idempotent, so calling it here costs nothing when the
	// handler already did — and guarantees the briefs are populated for callers
	// that build the request directly, like the scheduler.
	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("blog_svc: %w", err)
	}

	startedAt := time.Now()
	run := &model.BlogRun{
		ID:          uuid.New(),
		OrgID:       orgID,
		AgentID:     &agent.ID,
		TriggeredBy: triggeredBy,
		Source:      source,
		Status:      model.BlogRunStatusRunning,
		Briefs:      req.Briefs,
		Topics:      req.Topics,
		Steps:       initialSteps(req.Trending),
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

// recentTopicWindow is how far back a run looks to avoid repeating itself.
//
// Two weeks rather than a few days: a story stays on the front page for the
// better part of a week, and the failure this prevents — a daily schedule
// publishing the same article five times — is far more damaging than
// occasionally skipping a subject that deserved a second look.
const recentTopicWindow = 14 * 24 * time.Hour

// discoverBriefs finds subjects for a run that was not given any.
func (s *blogService) discoverBriefs(
	ctx context.Context,
	run *model.BlogRun,
	req model.GenerateBlogRequest,
	tracker *stepTracker,
) ([]model.BlogBrief, error) {
	if s.research == nil {
		return nil, fmt.Errorf("blog_svc: topic discovery is not configured")
	}
	tracker.advance(model.BlogStepDiscovering, "Finding what is trending", model.AgentNameResearcher)

	// What this org has published recently is what discovery must not repeat.
	// A failure here degrades the run to "might repeat itself" rather than
	// stopping it — losing an article to a database hiccup is the worse trade.
	avoid, err := s.repo.RecentTopics(ctx, run.OrgID, time.Now().Add(-recentTopicWindow))
	if err != nil {
		s.logger.Warn("blog_svc: could not load recent topics, discovery may repeat itself",
			zap.Error(err))
		avoid = nil
	}

	count := req.ResolvedTrendingCount(blog.HardMaxArticles)
	topics, err := s.research.Discover(ctx, run.OrgID, research.DiscoverRequest{
		Count: count,
		Avoid: avoid,
		Model: req.Model,
	}, func(_, detail string) {
		tracker.advance(model.BlogStepDiscovering, detail, model.AgentNameResearcher)
	})
	if err != nil {
		return nil, err
	}

	briefs := make([]model.BlogBrief, 0, len(topics))
	for _, t := range topics {
		briefs = append(briefs, model.BlogBrief{Topic: t.Topic, Context: t.Context})
	}
	if len(briefs) == 0 {
		return nil, fmt.Errorf("blog_svc: discovery returned no topics")
	}

	tracker.advance(model.BlogStepDiscovering,
		fmt.Sprintf("Chose %d topic(s)", len(briefs)), model.AgentNameResearcher)
	s.logger.Info("blog: discovered topics for a trending run",
		zap.String("blog_run_id", run.ID.String()),
		zap.Int("count", len(briefs)), zap.Int("avoided", len(avoid)))

	return briefs, nil
}

// failRun records a terminal failure on a run. Shared by the discovery and
// generation paths so the two cannot record it differently.
func (s *blogService) failRun(
	ctx context.Context,
	run *model.BlogRun,
	tracker *stepTracker,
	cause error,
	log *zap.Logger,
	agent *model.Agent,
) {
	tracker.fail(cause)
	run.Steps = tracker.steps
	completedAt := time.Now()
	run.CompletedAt = &completedAt
	msg := cause.Error()
	run.Status = model.BlogRunStatusFailed
	run.ErrorMessage = &msg
	if uerr := s.repo.Update(ctx, run); uerr != nil {
		log.Error("blog_svc: failed to record failure", zap.Error(uerr))
	}
	// Leave the agent active — 'failed' is a property of the run, and the
	// board reads the run, not the agent row.
	s.setAgentStatus(ctx, agent.ID, "active")
	log.Error("blog: run failed", zap.Error(cause))
}

func (s *blogService) runGeneration(run *model.BlogRun, agent *model.Agent, req model.GenerateBlogRequest) {
	ctx := context.Background()
	log := s.logger.With(zap.String("blog_run_id", run.ID.String()))

	tracker := &stepTracker{runID: run.ID, steps: run.Steps, repo: s.repo, logger: s.logger}
	s.setAgentStatus(ctx, agent.ID, "active")

	// A trending run has no subject yet — find one before anything else, and
	// record it on the run so what it chose is visible while it writes rather
	// than only once it finishes.
	if req.Trending {
		briefs, derr := s.discoverBriefs(ctx, run, req, tracker)
		if derr != nil {
			s.failRun(ctx, run, tracker, derr, log, agent)
			return
		}
		req.Briefs = briefs
		req.Normalize()
		run.Briefs, run.Topics = req.Briefs, req.Topics
		// Update does not write briefs — it is the terminal write — so the
		// discovered subject needs its own narrow one. Without it the run
		// finishes with an empty topic list, which leaves the page headerless
		// and makes Retry refuse the run for having nothing to retry.
		if uerr := s.repo.UpdateBriefs(ctx, run.ID, run.Briefs, run.Topics); uerr != nil {
			log.Warn("blog_svc: failed to record discovered topics", zap.Error(uerr))
		}
	}

	articles, err := s.runner.Generate(ctx, blog.GenerateRequest{
		OrgID:       run.OrgID,
		Briefs:      req.Briefs,
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
		refs := a.References
		if refs == nil {
			refs = []model.BlogReference{}
		}
		persisted = append(persisted, model.BlogArticle{
			ID: id, RunID: run.ID, OrgID: run.OrgID,
			Topic: a.Topic, Title: a.Title, Slug: a.Slug, Path: a.Path,
			References: refs,
			Markdown:   a.Markdown, HTML: a.HTML, WordCount: a.WordCount,
		})
		summaries = append(summaries, model.BlogRunArticle{
			ID: id, Topic: a.Topic, Title: a.Title, Slug: a.Slug, Path: a.Path,
			WordCount: a.WordCount, ReferenceCount: len(refs),
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
		return nil, fmt.Errorf("blog_svc: publishing is not configured (OPSAPI_BASE_URL, OPSAPI_TOKEN and OPSAPI_NAMESPACE must all be set)")
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
	if run.PublishedAt != nil {
		return nil, fmt.Errorf("blog_svc: run has already been published")
	}

	stored, err := s.repo.ListArticlesByRun(ctx, runID)
	if err != nil {
		return nil, err
	}
	if len(stored) == 0 {
		return nil, fmt.Errorf("blog_svc: run has no articles to publish")
	}

	// articleIDs maps slug back to the stored row, so the publish result — which
	// travels through the blog package and knows nothing of our IDs — can be
	// written back to the right articles. Slugs are de-duplicated within a run
	// at generation time, so they are unique here.
	//
	// Articles that already carry a post UUID are skipped. Publishing is not
	// atomic — a batch can fail on its fourth article with three drafts already
	// in the CMS — and the run stays unpublished so the user can retry. Without
	// this filter that retry would post the first three a second time, and the
	// CMS has no way to know they are the same article.
	articleIDs := make(map[string]uuid.UUID, len(stored))
	articles := make([]blog.GeneratedArticle, 0, len(stored))
	alreadyPosted := 0
	for _, a := range stored {
		if a.PostUUID != nil && *a.PostUUID != "" {
			alreadyPosted++
			continue
		}
		articleIDs[a.Slug] = a.ID
		articles = append(articles, blog.GeneratedArticle{
			Topic: a.Topic, Slug: a.Slug, Path: a.Path,
			Markdown: a.Markdown, HTML: a.HTML, WordCount: a.WordCount,
		})
	}

	// Everything already reached the CMS on an earlier attempt and only the
	// run's own bookkeeping was left undone. Finish that rather than reporting
	// "nothing to publish", which would read as a failure.
	if len(articles) == 0 {
		s.logger.Info("blog_svc: all articles already posted, finalising run",
			zap.String("blog_run_id", run.ID.String()), zap.Int("articles", alreadyPosted))
		return s.finalizePublished(ctx, run, s.runner.CMSNamespace(), time.Now())
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

	posted := make([]model.BlogArticlePost, 0, len(result.Posts))
	for _, p := range result.Posts {
		id, ok := articleIDs[p.Slug]
		if !ok {
			// Only reachable if the runner invented a slug we never sent. The
			// draft exists either way, so log it rather than failing a publish
			// that already happened.
			s.logger.Warn("blog_svc: published post has no matching article",
				zap.String("slug", p.Slug), zap.String("post_uuid", p.PostUUID))
			continue
		}
		posted = append(posted, model.BlogArticlePost{
			ArticleID: id, PostUUID: p.PostUUID, Status: p.Status,
		})
	}
	if err := s.repo.MarkArticlesPosted(ctx, posted); err != nil {
		// The drafts are in the CMS; losing our record of where is worth a loud
		// log but not an error that suggests the publish did not happen. It does
		// mean a later retry cannot tell these were already sent, so say so.
		s.logger.Error("blog_svc: failed to record posted articles — a retry would duplicate them",
			zap.Error(err))
	}

	return s.finalizePublished(ctx, run, result.Namespace, result.PublishedAt)
}

// finalizePublished stamps a run as published and persists it. Shared by the
// normal path and the already-posted path so the two cannot drift.
func (s *blogService) finalizePublished(
	ctx context.Context, run *model.BlogRun, namespace string, at time.Time,
) (*model.BlogRun, error) {
	run.CMSNamespace = &namespace
	run.PublishedAt = &at
	run.ErrorMessage = nil
	if err := s.repo.Update(ctx, run); err != nil {
		return nil, fmt.Errorf("blog_svc: persist publish: %w", err)
	}
	return run, nil
}

// Delete removes a run. Drafts already sent to the CMS are deliberately left
// alone: they live in another system that has its own notion of ownership, and
// silently deleting someone's CMS content because they tidied up a list here
// would be a surprise. Forgetting the run is the local action.
func (s *blogService) Delete(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) error {
	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return err
	}
	if run.OrgID != orgID {
		return fmt.Errorf("blog_svc: run does not belong to this organization")
	}
	if run.Status == model.BlogRunStatusRunning {
		return fmt.Errorf("blog_svc: cannot delete a run while it is still writing")
	}
	return s.repo.Delete(ctx, runID)
}

// Retry restarts a failed run using the topics it was created with.
//
// It reuses the same run rather than creating a second one: a retry is another
// attempt at the same request, and spawning a new card per attempt would leave
// the failures behind as litter — which is what made deletion necessary in the
// first place.
func (s *blogService) Retry(ctx context.Context, orgID uuid.UUID, runID uuid.UUID) (*model.BlogRun, error) {
	if s.runner == nil {
		return nil, fmt.Errorf("blog_svc: generator not configured")
	}

	run, err := s.repo.GetByID(ctx, runID)
	if err != nil {
		return nil, err
	}
	if run.OrgID != orgID {
		return nil, fmt.Errorf("blog_svc: run does not belong to this organization")
	}
	if run.Status != model.BlogRunStatusFailed {
		return nil, fmt.Errorf("blog_svc: only a failed run can be retried (status is %q)", run.Status)
	}
	if len(run.Briefs) == 0 {
		return nil, fmt.Errorf("blog_svc: run has no topics to retry")
	}

	agent, err := s.EnsureArticleWriter(ctx, orgID)
	if err != nil {
		return nil, err
	}

	// A run can fail after its articles were written — storing them is a
	// separate step that can fail on its own. Clear them so a retry cannot
	// leave two attempts' output on the same run.
	if err := s.repo.DeleteArticlesByRun(ctx, runID); err != nil {
		return nil, err
	}

	startedAt := time.Now()
	run.Status = model.BlogRunStatusRunning
	run.AgentID = &agent.ID
	// A retry replays the topics the run already settled on, so it never
	// rediscovers — even if the original run found them by discovery. Retrying
	// into a different subject would make the button mean something else.
	run.Steps = initialSteps(false)
	run.Articles = []model.BlogRunArticle{}
	run.ErrorMessage = nil
	run.StartedAt = &startedAt
	run.CompletedAt = nil

	if err := s.repo.Update(ctx, run); err != nil {
		return nil, fmt.Errorf("blog_svc: reset run for retry: %w", err)
	}
	// Update does not write steps back to their reset state on its own path for
	// a fresh run, so stamp the trace explicitly before work starts.
	if err := s.repo.UpdateSteps(ctx, run.ID, run.Steps); err != nil {
		return nil, fmt.Errorf("blog_svc: reset steps for retry: %w", err)
	}

	// Retry replays the briefs, not just the topics: the context is half the
	// instruction, and a retry that dropped it would write a different article
	// than the one that was asked for.
	req := model.GenerateBlogRequest{Briefs: run.Briefs}
	req.Normalize()
	if run.Model != nil {
		req.Model = *run.Model
	}
	go s.runGeneration(run, agent, req)

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
