package service

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/blog"
	"github.com/jobshout/server/internal/model"
)

// lockedStore is runStore with a hand for the test to play the other replica.
type lockedStore struct {
	runStore
}

// otherReplica changes the stored row the way another API pod would.
func (s *lockedStore) otherReplica(change func(run *model.BlogRun)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := *s.run
	change(&next)
	s.run = &next
}
func (s *lockedStore) snapshot() model.BlogRun {
	s.mu.Lock()
	defer s.mu.Unlock()
	return *s.run
}

// startBlockedAttempt runs generation for attempt 1 of a run whose research
// never returns, as the replica that owns the writing goroutine. done closes
// when that goroutine gives up.
func startBlockedAttempt(t *testing.T) (*lockedStore, <-chan struct{}) {
	t.Helper()
	prev := heartbeatInterval
	heartbeatInterval = 10 * time.Millisecond
	t.Cleanup(func() { heartbeatInterval = prev })

	stored := aRun(uuid.New(), model.BlogRunStatusRunning, "one")
	stored.Attempt = 1
	store := &lockedStore{runStore: runStore{run: stored}}
	svc := &blogService{repo: store, logger: zap.NewNop()}
	svc.runner = blog.NewRunner(blog.Config{}, idleLLM{}, nil, blockingResearcher{}, zap.NewNop())

	mine := *stored // the writer's own copy, as loaded by its replica
	done := make(chan struct{})
	go func() {
		defer close(done)
		svc.runGeneration(context.Background(), &mine, &model.Agent{ID: uuid.New()},
			model.GenerateBlogRequest{Briefs: mine.Briefs})
	}()
	return store, done
}

func waitDone(t *testing.T, done <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("%s: the writing goroutine kept going", what)
	}
}

// Cancel handled by the other replica only marks the row: it has no handle on
// the goroutine doing the writing. That goroutine has to notice and stop.
func TestAttempt_CancelFromAnotherReplicaStopsTheWriter(t *testing.T) {
	store, done := startBlockedAttempt(t)

	store.otherReplica(func(run *model.BlogRun) { run.Status = model.BlogRunStatusCancelled })

	waitDone(t, done, "cancelled elsewhere")
	if got := store.snapshot().Status; got != model.BlogRunStatusCancelled {
		t.Fatalf("status = %q, want it left cancelled", got)
	}
}

// After cancel-then-retry on another replica the row is running again, on a new
// attempt. The old writer must stop and must not settle or trace over it.
func TestAttempt_StaleWriterCannotTouchARetriedRun(t *testing.T) {
	store, done := startBlockedAttempt(t)

	fresh := initialSteps(false)
	store.otherReplica(func(run *model.BlogRun) {
		run.Attempt = 2
		run.Status = model.BlogRunStatusRunning
		run.Steps = fresh
		run.ErrorMessage = nil
	})

	waitDone(t, done, "retried elsewhere")
	got := store.snapshot()
	if got.Status != model.BlogRunStatusRunning || got.Attempt != 2 {
		t.Fatalf("run = %s attempt %d, want the retry left running on attempt 2", got.Status, got.Attempt)
	}
	if got.ErrorMessage != nil {
		t.Errorf("the stale writer recorded %q on the retried run", *got.ErrorMessage)
	}
	for _, step := range got.Steps {
		if step.Status == model.StepStatusFailed {
			t.Errorf("the stale writer failed step %q of the retried run", step.Key)
		}
	}
}

// An article finished by a superseded attempt is not stored on the run.
func TestAttempt_StaleWriterCannotStoreAnArticle(t *testing.T) {
	stored := aRun(uuid.New(), model.BlogRunStatusRunning, "one")
	stored.Attempt = 2
	svc, store := newLifecycleSvc(stored)

	stale := *stored
	stale.Attempt = 1
	err := svc.persistArticle(&stale, blog.GeneratedArticle{Topic: "one", Title: "One", Slug: "one"})
	if err == nil {
		t.Fatal("a superseded attempt stored an article")
	}
	if len(store.storedArticles) != 0 || len(store.run.Articles) != 0 {
		t.Fatalf("articles were written: %d rows, %d summaries", len(store.storedArticles), len(store.run.Articles))
	}
}

// A retry replaces the run's entry in the active set before the previous
// attempt's goroutine has unwound; that goroutine must not remove it.
func TestUntrack_LeavesASuccessorInPlace(t *testing.T) {
	svc := &blogService{}
	id := uuid.New()
	old, next := &trackedRun{}, &trackedRun{}
	svc.active = map[uuid.UUID]*trackedRun{id: next}

	svc.untrack(id, old)
	if !svc.isActive(id) {
		t.Fatal("the previous attempt untracked its successor")
	}
	svc.untrack(id, next)
	if svc.isActive(id) {
		t.Fatal("the run's own goroutine could not untrack it")
	}
}

// The same Retry arriving twice — a double click, or once on each replica —
// starts one attempt.
func TestRetry_SecondRetryOfTheSameRunIsRefused(t *testing.T) {
	org := uuid.New()
	run := aRun(org, model.BlogRunStatusFailed, "one")
	run.Attempt = 1
	svc, store := newLifecycleSvc(run)
	svc.runner = nonNilRunner()
	svc.agentRepo = &retryAgents{agent: &model.Agent{ID: uuid.New(), OrgID: org}}

	// The other replica read the same failed row, then won the retry.
	seenByOther := *run
	if won, _ := store.BeginRetry(context.Background(), run.ID, seenByOther.Attempt); !won {
		t.Fatal("first retry did not win")
	}

	err := svc.claimRetry(context.Background(), &seenByOther)
	if err == nil || !strings.Contains(err.Error(), "already being retried") {
		t.Fatalf("second retry = %v, want it refused", err)
	}
	if store.run.Attempt != 2 {
		t.Errorf("attempt = %d, want 2", store.run.Attempt)
	}
}
