package service

import (
	"context"
	"strings"
	"testing"

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

func newLifecycleSvc(run *model.BlogRun) (*blogService, *runStore) {
	store := &runStore{run: run}
	return &blogService{repo: store, logger: zap.NewNop()}, store
}

// nonNilRunner satisfies Retry's "generator configured" check. The guards under
// test all return before anything reaches it.
func nonNilRunner() *blog.Runner {
	return blog.NewRunner(blog.Config{}, nil, nil, zap.NewNop())
}

func aRun(orgID uuid.UUID, status string, topics ...string) *model.BlogRun {
	return &model.BlogRun{ID: uuid.New(), OrgID: orgID, Status: status, Topics: topics}
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
