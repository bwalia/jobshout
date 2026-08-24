package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// runStore is a repository stub holding one run, enough to exercise the guards
// that Delete and Retry apply before doing any work.
type runStore struct {
	repository.BlogRepository
	run             *model.BlogRun
	stale           []*model.BlogRun
	storedArticles  []model.BlogArticle
	deleted         bool
	articlesCleared bool
}

func (s *runStore) GetByID(_ context.Context, _ uuid.UUID) (*model.BlogRun, error) {
	return s.run, nil
}
func (s *runStore) Delete(_ context.Context, _ uuid.UUID) error {
	s.deleted = true
	return nil
}
func (s *runStore) DeleteArticlesByRun(_ context.Context, _ uuid.UUID) error {
	s.articlesCleared = true
	return nil
}
func (s *runStore) Update(_ context.Context, run *model.BlogRun) error {
	copy := *run
	s.run = &copy
	return nil
}
func (s *runStore) UpdateSteps(_ context.Context, _ uuid.UUID, steps []model.BlogStep) error {
	if s.run != nil {
		s.run.Steps = append([]model.BlogStep(nil), steps...)
	}
	return nil
}
func (s *runStore) TouchHeartbeat(_ context.Context, _ uuid.UUID) error { return nil }
func (s *runStore) ListStaleRunning(_ context.Context, _ time.Time) ([]*model.BlogRun, error) {
	if s.stale == nil {
		return nil, nil
	}
	return s.stale, nil
}
func (s *runStore) CreateArticles(_ context.Context, articles []model.BlogArticle) error {
	s.storedArticles = append(s.storedArticles, articles...)
	return nil
}
func (s *runStore) ListArticlesByRun(_ context.Context, _ uuid.UUID) ([]model.BlogArticle, error) {
	return append([]model.BlogArticle(nil), s.storedArticles...), nil
}
func (s *runStore) UpdateArticles(_ context.Context, _ uuid.UUID, articles []model.BlogRunArticle) error {
	if s.run != nil {
		s.run.Articles = append([]model.BlogRunArticle(nil), articles...)
	}
	return nil
}

func newLifecycleSvc(run *model.BlogRun) (*blogService, *runStore) {
	store := &runStore{run: run}
	return &blogService{repo: store, logger: zap.NewNop()}, store
}

// nonNilRunner satisfies Retry's "generator configured" check. The guards under
// test all return before anything reaches it.
func nonNilRunner() *blog.Runner {
	return blog.NewRunner(blog.Config{}, nil, nil, nil, zap.NewNop())
}

func aRun(orgID uuid.UUID, status string, topics ...string) *model.BlogRun {
	// Briefs are what Retry replays now; the repository derives them from
	// topics on read, so a run built by hand has to do the same.
	briefs := make([]model.BlogBrief, 0, len(topics))
	for _, t := range topics {
		briefs = append(briefs, model.BlogBrief{Topic: t})
	}
	return &model.BlogRun{ID: uuid.New(), OrgID: orgID, Status: status, Topics: topics, Briefs: briefs}
}

func TestDelete_RefusesAnotherOrgsRun(t *testing.T) {
	owner, intruder := uuid.New(), uuid.New()
	svc, store := newLifecycleSvc(aRun(owner, model.BlogRunStatusFailed, "t"))

	if err := svc.Delete(context.Background(), intruder, store.run.ID); err == nil {
		t.Fatal("expected a cross-organization delete to be refused")
	}
	if store.deleted {
		t.Error("the run was deleted despite belonging to another organization")
	}
}

// Deleting mid-flight would leave the background generation writing to a row
// that no longer exists.
func TestDelete_RefusesWhileRunning(t *testing.T) {
	org := uuid.New()
	svc, store := newLifecycleSvc(aRun(org, model.BlogRunStatusRunning, "t"))

	err := svc.Delete(context.Background(), org, store.run.ID)
	if err == nil {
		t.Fatal("expected delete to be refused while the run is writing")
	}
	if !strings.Contains(err.Error(), "still writing") {
		t.Errorf("error should say why, got: %v", err)
	}
	if store.deleted {
		t.Error("the run was deleted while still running")
	}
}

func TestDelete_RemovesAFinishedRun(t *testing.T) {
	org := uuid.New()
	svc, store := newLifecycleSvc(aRun(org, model.BlogRunStatusCompleted, "t"))

	if err := svc.Delete(context.Background(), org, store.run.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !store.deleted {
		t.Error("Delete did not reach the repository")
	}
}

func TestRetry_OnlyFailedRuns(t *testing.T) {
	org := uuid.New()
	for _, status := range []string{
		model.BlogRunStatusCompleted,
		model.BlogRunStatusRunning,
		model.BlogRunStatusPending,
	} {
		t.Run(status, func(t *testing.T) {
			svc, store := newLifecycleSvc(aRun(org, status, "t"))
			svc.runner = nonNilRunner()

			_, err := svc.Retry(context.Background(), org, store.run.ID)
			if err == nil {
				t.Fatalf("expected retry to be refused for status %q", status)
			}
			if !strings.Contains(err.Error(), "only a failed run") {
				t.Errorf("error should explain the rule, got: %v", err)
			}
			if store.articlesCleared {
				t.Error("a refused retry must not touch the run's articles")
			}
		})
	}
}

func TestRetry_RefusesAnotherOrgsRun(t *testing.T) {
	owner, intruder := uuid.New(), uuid.New()
	svc, store := newLifecycleSvc(aRun(owner, model.BlogRunStatusFailed, "t"))
	svc.runner = nonNilRunner()

	if _, err := svc.Retry(context.Background(), intruder, store.run.ID); err == nil {
		t.Fatal("expected a cross-organization retry to be refused")
	}
	if store.articlesCleared {
		t.Error("a refused retry must not touch the run's articles")
	}
}

// A run with no topics cannot be re-run — there is nothing to ask the LLM for.
func TestRetry_RefusesRunWithNoTopics(t *testing.T) {
	org := uuid.New()
	svc, store := newLifecycleSvc(aRun(org, model.BlogRunStatusFailed))
	svc.runner = nonNilRunner()

	if _, err := svc.Retry(context.Background(), org, store.run.ID); err == nil {
		t.Fatal("expected retry to be refused with no topics")
	}
}

func TestCancel_RefusesAnotherOrgsRun(t *testing.T) {
	owner, intruder := uuid.New(), uuid.New()
	svc, store := newLifecycleSvc(aRun(owner, model.BlogRunStatusRunning, "t"))

	if _, err := svc.Cancel(context.Background(), intruder, store.run.ID); err == nil {
		t.Fatal("expected a cross-organization cancel to be refused")
	}
	if store.run.Status != model.BlogRunStatusRunning {
		t.Errorf("status = %q, want still running", store.run.Status)
	}
}

func TestCancel_RefusesFinishedRun(t *testing.T) {
	org := uuid.New()
	svc, store := newLifecycleSvc(aRun(org, model.BlogRunStatusCompleted, "t"))

	_, err := svc.Cancel(context.Background(), org, store.run.ID)
	if err == nil {
		t.Fatal("expected cancel to be refused for a finished run")
	}
	if !strings.Contains(err.Error(), "only a running run") {
		t.Errorf("error should explain the rule, got: %v", err)
	}
}

// A run left `running` after the writer died (a deploy) has no goroutine in
// this process. Cancel must still mark it failed, otherwise Retry and Delete
// stay locked out.
func TestCancel_MarksOrphanFailed(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "t")
	run.Steps = []model.BlogStep{
		{Key: model.BlogStepGenerating, Label: "Writing", Status: model.StepStatusRunning},
	}
	svc, store := newLifecycleSvc(run)

	got, err := svc.Cancel(context.Background(), org, store.run.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if got.Status != model.BlogRunStatusFailed {
		t.Errorf("status = %q, want failed", got.Status)
	}
	if got.ErrorMessage == nil || *got.ErrorMessage != errRunCancelled.Error() {
		t.Errorf("error_message = %v, want %q", got.ErrorMessage, errRunCancelled)
	}
	step := got.Steps[0]
	if step.Status != model.StepStatusFailed {
		t.Errorf("step status = %q, want failed", step.Status)
	}
}

func TestCancel_AbortsTrackedRun(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "t")
	svc, store := newLifecycleSvc(run)

	cancelled := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	svc.active = map[uuid.UUID]*trackedRun{
		run.ID: {
			cancel: func() {
				cancel()
				close(cancelled)
			},
		},
	}

	if _, err := svc.Cancel(context.Background(), org, store.run.ID); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	select {
	case <-cancelled:
	default:
		t.Fatal("Cancel did not invoke the generation cancel func")
	}
	if ctx.Err() == nil {
		t.Fatal("tracked context was not cancelled")
	}
}

func TestInterruptAll_FailsTrackedRuns(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "t")
	run.Steps = []model.BlogStep{
		{Key: model.BlogStepGenerating, Label: "Writing", Status: model.StepStatusRunning},
	}
	svc, store := newLifecycleSvc(run)

	ctx, cancel := context.WithCancel(context.Background())
	svc.active = map[uuid.UUID]*trackedRun{
		run.ID: {cancel: cancel},
	}

	svc.InterruptAll(errRunInterrupted)

	if ctx.Err() == nil {
		t.Fatal("InterruptAll did not cancel the tracked context")
	}
	if store.run.Status != model.BlogRunStatusFailed {
		t.Errorf("status = %q, want failed", store.run.Status)
	}
	if store.run.ErrorMessage == nil || *store.run.ErrorMessage != errRunInterrupted.Error() {
		t.Errorf("error_message = %v, want %q", store.run.ErrorMessage, errRunInterrupted)
	}
	if !svc.stopping {
		t.Error("InterruptAll should refuse new generations")
	}
}

func TestInterruptAll_DoesNotClobberCompleted(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusCompleted, "t")
	svc, store := newLifecycleSvc(run)

	_, cancel := context.WithCancel(context.Background())
	svc.active = map[uuid.UUID]*trackedRun{
		run.ID: {cancel: cancel},
	}

	svc.InterruptAll(errRunInterrupted)

	if store.run.Status != model.BlogRunStatusCompleted {
		t.Errorf("status = %q, want still completed", store.run.Status)
	}
}

func TestBeginGeneration_RefusesWhenStopping(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "t")
	svc, _ := newLifecycleSvc(run)
	svc.stopping = true

	err := svc.beginGeneration(run, &model.Agent{ID: uuid.New()}, model.GenerateBlogRequest{})
	if !errors.Is(err, errRunStopping) {
		t.Fatalf("beginGeneration = %v, want errRunStopping", err)
	}
}

func TestReapOrphans_FailsStaleNotActive(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "t")
	svc, store := newLifecycleSvc(run)
	store.stale = []*model.BlogRun{run}

	n, err := svc.ReapOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("reaped %d, want 1", n)
	}
	if store.run.Status != model.BlogRunStatusFailed {
		t.Errorf("status = %q, want failed", store.run.Status)
	}
	if store.run.ErrorMessage == nil || *store.run.ErrorMessage != errRunOrphaned.Error() {
		t.Errorf("error_message = %v, want %q", store.run.ErrorMessage, errRunOrphaned.Error())
	}
}

func TestReapOrphans_SkipsLiveTrackedRun(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "t")
	svc, store := newLifecycleSvc(run)
	store.stale = []*model.BlogRun{run}
	_, cancel := context.WithCancel(context.Background())
	defer cancel()
	svc.active = map[uuid.UUID]*trackedRun{run.ID: {cancel: cancel}}

	n, err := svc.ReapOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("reaped %d, want 0 (live run)", n)
	}
	if store.run.Status != model.BlogRunStatusRunning {
		t.Errorf("status = %q, want still running", store.run.Status)
	}
}

func TestReapOrphans_CompletedUntouched(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusCompleted, "t")
	svc, store := newLifecycleSvc(run)
	store.stale = []*model.BlogRun{run}

	n, err := svc.ReapOrphans(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("reaped %d, want 0 (completed is not an orphan)", n)
	}
	if store.run.Status != model.BlogRunStatusCompleted {
		t.Errorf("status = %q, want still completed", store.run.Status)
	}
}

func TestBlogReconciler_TickReaps(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "t")
	svc, store := newLifecycleSvc(run)
	store.stale = []*model.BlogRun{run}

	rc := NewBlogReconciler(svc, time.Minute, zap.NewNop())
	if err := rc.Tick(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.run.Status != model.BlogRunStatusFailed {
		t.Errorf("status = %q, want failed", store.run.Status)
	}
}

func TestPersistArticle_ThenBriefFailure_CompletesWithError(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusRunning, "one", "two")
	svc, store := newLifecycleSvc(run)

	err := svc.persistArticle(run, blog.GeneratedArticle{
		Topic: "one", Title: "One", Slug: "one", Path: "content/blogs/one.md",
		Markdown: "# One\n\nHi.", HTML: "<p>Hi</p>", WordCount: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.storedArticles) != 1 {
		t.Fatalf("stored %d articles, want 1", len(store.storedArticles))
	}

	tracker := &stepTracker{
		runID: run.ID, steps: initialSteps(false), repo: store, logger: zap.NewNop(),
	}
	if !svc.finishSuccessfulRun(run, tracker, errors.New(`blog: "two": research boom`), zap.NewNop(), nil) {
		t.Fatal("finishSuccessfulRun returned false")
	}
	if store.run.Status != model.BlogRunStatusCompleted {
		t.Errorf("status = %q, want completed", store.run.Status)
	}
	if store.run.ErrorMessage == nil || !strings.Contains(*store.run.ErrorMessage, "two") {
		t.Errorf("error_message = %v, want it to name the failed brief", store.run.ErrorMessage)
	}
	if len(store.run.Articles) != 1 || store.run.Articles[0].Topic != "one" {
		t.Errorf("articles = %+v, want the stored brief only", store.run.Articles)
	}
}

func TestRetry_RefusesWhenEveryTopicHasAnArticle(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusFailed, "t")
	svc, store := newLifecycleSvc(run)
	svc.runner = nonNilRunner()
	store.storedArticles = []model.BlogArticle{{
		ID: uuid.New(), RunID: run.ID, OrgID: org, Topic: "t", Title: "T",
	}}

	_, err := svc.Retry(context.Background(), org, run.ID)
	if err == nil || !strings.Contains(err.Error(), "already has an article") {
		t.Fatalf("Retry = %v, want refusal because every topic is stored", err)
	}
	if store.articlesCleared {
		t.Error("retry must not delete stored articles")
	}
}
