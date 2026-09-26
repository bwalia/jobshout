package scheduler

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// pauseRepo records what the dispatcher did with a task after running it.
type pauseRepo struct {
	repository.SchedulerRepository
	runs     []*model.ScheduledTaskRun
	statuses []string
	nextRuns int
}

func (p *pauseRepo) CreateRun(_ context.Context, run *model.ScheduledTaskRun) error {
	p.runs = append(p.runs, run)
	return nil
}
func (p *pauseRepo) IncrementRunCount(context.Context, uuid.UUID) error { return nil }
func (p *pauseRepo) SetNextRunAt(context.Context, uuid.UUID, time.Time) error {
	p.nextRuns++
	return nil
}
func (p *pauseRepo) UpdateTask(_ context.Context, _ uuid.UUID, req model.UpdateScheduledTaskRequest) (*model.ScheduledTask, error) {
	if req.Status != nil {
		p.statuses = append(p.statuses, *req.Status)
	}
	return &model.ScheduledTask{}, nil
}

// An article schedule saved with no input fails the same way every time it
// fires. It must be paused with the reason recorded, not rescheduled.
func TestRunOne_PausesABlogScheduleWithNothingToWrite(t *testing.T) {
	repo := &pauseRepo{}
	r := NewRunner(repo, nil, nil, nil, nil, zap.NewNop())
	cron := "0 */5 * * *"
	task := model.ScheduledTask{
		ID: uuid.New(), Name: "Article Writer Schedule", TaskType: "blog",
		ScheduleType: "cron", CronExpression: &cron,
	}

	r.runOne(context.Background(), task)

	if len(repo.runs) != 1 || repo.runs[0].Status != "failed" || repo.runs[0].ErrorMessage == nil {
		t.Fatalf("runs = %+v, want one failed run", repo.runs)
	}
	if msg := *repo.runs[0].ErrorMessage; !strings.Contains(msg, "Anything trending") {
		t.Errorf("error %q does not say how to fix the schedule", msg)
	}
	if len(repo.statuses) != 1 || repo.statuses[0] != "paused" {
		t.Errorf("statuses = %v, want the task paused", repo.statuses)
	}
	if repo.nextRuns != 0 {
		t.Error("a paused task was rescheduled")
	}
}

func TestBlogRequestFromInput_NothingToWriteIsMisconfigured(t *testing.T) {
	_, err := blogRequestFromInput(nil)
	if !errors.Is(err, errTaskMisconfigured) {
		t.Fatalf("err = %v, want errTaskMisconfigured", err)
	}
}
