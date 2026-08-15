package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// stepRecorder captures what the tracker persists, so the tests assert on the
// trace the API would actually serve rather than on in-memory state.
type stepRecorder struct {
	repository.BlogRepository
	last  []model.BlogStep
	calls int
}

func (r *stepRecorder) UpdateSteps(_ context.Context, _ uuid.UUID, steps []model.BlogStep) error {
	r.last = append([]model.BlogStep(nil), steps...)
	r.calls++
	return nil
}

func newTracker() (*stepTracker, *stepRecorder) {
	rec := &stepRecorder{}
	return &stepTracker{
		runID:  uuid.New(),
		steps:  initialSteps(false),
		repo:   rec,
		logger: zap.NewNop(),
	}, rec
}

func findStep(steps []model.BlogStep, key string) *model.BlogStep {
	for i := range steps {
		if steps[i].Key == key {
			return &steps[i]
		}
	}
	return nil
}

// Exactly one step may be running at a time: the agent board picks the running
// step out of the JSONB with a LIMIT 1, so a second one would make the card's
// label arbitrary.
func TestStepTracker_OneRunningStepAtATime(t *testing.T) {
	tr, _ := newTracker()

	tr.advance(model.BlogStepGenerating, "Writing 1/2 — a", model.AgentNameArticleWriter)
	tr.advance(model.BlogStepGenerated, "Generated 2 article(s)", model.AgentNameArticleWriter)

	running := 0
	for _, s := range tr.steps {
		if s.Status == model.StepStatusRunning {
			running++
		}
	}
	if running != 1 {
		t.Errorf("running steps = %d, want exactly 1", running)
	}
	if findStep(tr.steps, model.BlogStepGenerating).Status != model.StepStatusDone {
		t.Error("advancing did not close the previous step")
	}
}

// "generating" fires once per topic. Re-entering it must not make it look
// finished, and must not restart its clock — the phase spans every topic.
func TestStepTracker_ReenteredStepStaysRunning(t *testing.T) {
	tr, _ := newTracker()

	tr.advance(model.BlogStepGenerating, "Writing 1/3 — a", model.AgentNameArticleWriter)
	first := findStep(tr.steps, model.BlogStepGenerating).StartedAt
	if first == nil {
		t.Fatal("first entry did not set StartedAt")
	}

	tr.advance(model.BlogStepGenerating, "Writing 2/3 — b", model.AgentNameArticleWriter)
	tr.advance(model.BlogStepGenerating, "Writing 3/3 — c", model.AgentNameArticleWriter)

	step := findStep(tr.steps, model.BlogStepGenerating)
	if step.Status != model.StepStatusRunning {
		t.Errorf("status = %q, want still running", step.Status)
	}
	if step.CompletedAt != nil {
		t.Error("a still-running step must not carry a completion time")
	}
	if !step.StartedAt.Equal(*first) {
		t.Error("re-entering restarted the clock; duration would only cover the last topic")
	}
	if step.Label != "Writing 3/3 — c" {
		t.Errorf("label = %q, want the newest one", step.Label)
	}
}

func TestStepTracker_FinishClosesRunningStep(t *testing.T) {
	tr, rec := newTracker()

	tr.advance(model.BlogStepGenerating, "Writing 1/1 — a", model.AgentNameArticleWriter)
	tr.finish()

	step := findStep(rec.last, model.BlogStepGenerating)
	if step.Status != model.StepStatusDone {
		t.Errorf("status = %q, want done", step.Status)
	}
	if step.CompletedAt == nil {
		t.Error("a finished step should record when it completed")
	}
}

// A failure marks the step that was running and leaves later steps pending, so
// the trace shows how far the run actually got.
func TestStepTracker_FailMarksOnlyTheRunningStep(t *testing.T) {
	tr, rec := newTracker()

	tr.advance(model.BlogStepGenerating, "Writing 1/1 — a", model.AgentNameArticleWriter)
	tr.fail(errors.New("ollama unreachable"))

	failed := findStep(rec.last, model.BlogStepGenerating)
	if failed.Status != model.StepStatusFailed {
		t.Errorf("status = %q, want failed", failed.Status)
	}
	if failed.Error == nil || *failed.Error != "ollama unreachable" {
		t.Errorf("error not recorded on the step: %v", failed.Error)
	}
	if later := findStep(rec.last, model.BlogStepGenerated); later.Status != model.StepStatusPending {
		t.Errorf("later step = %q, want it left pending", later.Status)
	}
}

// An unknown key appends rather than being dropped, so a future pipeline phase
// still shows up in the trace without a coordinated change here.
func TestStepTracker_UnknownKeyIsAppended(t *testing.T) {
	tr, _ := newTracker()
	before := len(tr.steps)

	tr.advance("verifying", "Checking links", model.AgentNameArticleWriter)

	if len(tr.steps) != before+1 {
		t.Fatalf("steps = %d, want %d", len(tr.steps), before+1)
	}
	last := tr.steps[len(tr.steps)-1]
	if last.Key != "verifying" || last.Status != model.StepStatusRunning {
		t.Errorf("appended step = %+v", last)
	}
}

// Publishing appends its phases only when a publish is requested, so a run that
// is never published does not display steps it will never reach.
func TestPublishStepsAreSeparate(t *testing.T) {
	for _, s := range initialSteps(false) {
		if s.Key == model.BlogStepPublishing || s.Key == model.BlogStepPublished {
			t.Errorf("initialSteps must not contain publish phase %q", s.Key)
		}
	}
	keys := map[string]bool{}
	for _, s := range publishSteps() {
		keys[s.Key] = true
		if s.Status != model.StepStatusPending {
			t.Errorf("publish step %q should start pending", s.Key)
		}
	}
	for _, want := range []string{model.BlogStepPublishing, model.BlogStepPublished} {
		if !keys[want] {
			t.Errorf("publishSteps missing %q", want)
		}
	}
}
