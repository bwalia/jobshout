package service

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/research"
)

// blockingResearcher holds research until the run's context ends, standing in
// for a queued Ollama that never answers inside the budget.
type blockingResearcher struct{}

func (blockingResearcher) Research(ctx context.Context, _ uuid.UUID, _ research.Request, _ research.ProgressFunc) (*research.Brief, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

type idleLLM struct{}

func (idleLLM) Generate(ctx context.Context, _ llm.GenerateRequest) (*llm.GenerateResponse, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (idleLLM) ProviderName() string { return "idle" }

// runOutOfTime runs generation for a two-topic run whose budget expires while
// the second topic is still being researched. stored says whether the first
// article had already been saved by then.
func runOutOfTime(t *testing.T, stored bool, cancelFirst bool) *runStore {
	t.Helper()
	run := aRun(uuid.New(), model.BlogRunStatusRunning, "one", "two")
	if stored {
		run.Articles = []model.BlogRunArticle{{Topic: "one", Title: "One", Slug: "one"}}
	}
	svc, store := newLifecycleSvc(run)
	svc.runner = blog.NewRunner(blog.Config{}, idleLLM{}, nil, blockingResearcher{}, zap.NewNop())
	svc.active = map[uuid.UUID]*trackedRun{run.ID: {cancel: func() {}}}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if cancelFirst {
		svc.signalCancel(run.ID, errRunCancelled)
	}
	svc.runGeneration(ctx, run, &model.Agent{ID: uuid.New()}, model.GenerateBlogRequest{Briefs: run.Briefs})
	return store
}

// A budget that runs out after an article was stored must not strand that
// article on a failed run, where nothing can publish it.
func TestRunGeneration_TimeoutAfterStoredArticleCompletes(t *testing.T) {
	store := runOutOfTime(t, true, false)
	if store.run.Status != model.BlogRunStatusCompleted {
		t.Fatalf("status = %q, want completed", store.run.Status)
	}
	if store.run.ErrorMessage == nil {
		t.Fatal("error_message is empty; the timeout should still be recorded")
	}
}

func TestRunGeneration_TimeoutWithNothingStoredFails(t *testing.T) {
	store := runOutOfTime(t, false, false)
	if store.run.Status != model.BlogRunStatusFailed {
		t.Fatalf("status = %q, want failed", store.run.Status)
	}
}

// A person cancelling keeps the run cancelled even with an article stored.
func TestRunGeneration_CancelAfterStoredArticleStaysCancelled(t *testing.T) {
	store := runOutOfTime(t, true, true)
	if store.run.Status == model.BlogRunStatusCompleted {
		t.Fatal("a cancelled run was recorded as completed")
	}
}

type panickingResearcher struct{}

func (panickingResearcher) Research(context.Context, uuid.UUID, research.Request, research.ProgressFunc) (*research.Brief, error) {
	panic("boom")
}

// A panic in one run fails that run instead of taking the pod down.
func TestBeginGeneration_PanicFailsTheRun(t *testing.T) {
	run := aRun(uuid.New(), model.BlogRunStatusRunning, "one")
	svc, store := newLifecycleSvc(run)
	svc.runner = blog.NewRunner(blog.Config{}, idleLLM{}, nil, panickingResearcher{}, zap.NewNop())

	if err := svc.beginGeneration(run, &model.Agent{ID: uuid.New()}, model.GenerateBlogRequest{Briefs: run.Briefs}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for svc.isActive(run.ID) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if store.run.Status != model.BlogRunStatusFailed {
		t.Fatalf("status = %q, want failed", store.run.Status)
	}
}
