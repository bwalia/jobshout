package scheduler

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// runNowRepo records what RunNow touches so a manual fire cannot move the
// schedule's cadence or pause/complete it.
type runNowRepo struct {
	repository.SchedulerRepository
	created       int
	increments    int
	nextRunWrites int
	statuses      []string
}

func (r *runNowRepo) CreateRun(context.Context, *model.ScheduledTaskRun) error {
	r.created++
	return nil
}
func (r *runNowRepo) IncrementRunCount(context.Context, uuid.UUID) error {
	r.increments++
	return nil
}
func (r *runNowRepo) SetNextRunAt(context.Context, uuid.UUID, time.Time) error {
	r.nextRunWrites++
	return nil
}
func (r *runNowRepo) UpdateTask(_ context.Context, _ uuid.UUID, req model.UpdateScheduledTaskRequest) (*model.ScheduledTask, error) {
	if req.Status != nil {
		r.statuses = append(r.statuses, *req.Status)
	}
	return &model.ScheduledTask{}, nil
}

func TestRunNow_RecordsFailureWithoutAdvancingSchedule(t *testing.T) {
	cron := "0 9 * * *"
	next := time.Now().Add(time.Hour).Truncate(time.Second)
	repo := &runNowRepo{}
	runner := NewRunner(repo, nil, nil, nil, nil, zap.NewNop())
	task := model.ScheduledTask{
		ID:             uuid.New(),
		Name:           "Daily report",
		TaskType:       "agent",
		ScheduleType:   "cron",
		CronExpression: &cron,
		Status:         "active",
		NextRunAt:      &next,
		// No agent_id — same undirected failure the cron path would hit.
	}

	run, err := runner.RunNow(context.Background(), task)
	if err == nil {
		t.Fatal("expected error for undirected agent task")
	}
	if run == nil || run.Status != "failed" || run.ErrorMessage == nil {
		t.Fatalf("failed run record = %+v", run)
	}
	if repo.created != 1 || repo.increments != 1 {
		t.Fatalf("persist/count = %d/%d, want 1/1", repo.created, repo.increments)
	}
	if repo.nextRunWrites != 0 {
		t.Fatalf("manual run moved next_run_at (%d writes)", repo.nextRunWrites)
	}
	if len(repo.statuses) != 0 {
		t.Fatalf("manual run changed status to %v", repo.statuses)
	}
}

// A misconfigured article schedule is paused by the cron path. A manual
// Run once must not pause it — operators are testing after an edit, and
// Pause/Resume on the card is the deliberate control.
func TestRunNow_DoesNotPauseMisconfiguredBlog(t *testing.T) {
	cron := "0 */5 * * *"
	repo := &runNowRepo{}
	runner := NewRunner(repo, nil, nil, nil, nil, zap.NewNop())
	task := model.ScheduledTask{
		ID: uuid.New(), Name: "Article Writer Schedule", TaskType: "blog",
		ScheduleType: "cron", CronExpression: &cron,
	}

	run, err := runner.RunNow(context.Background(), task)
	if err == nil {
		t.Fatal("expected misconfigured error")
	}
	if run == nil || run.Status != "failed" {
		t.Fatalf("run = %+v", run)
	}
	if len(repo.statuses) != 0 {
		t.Fatalf("RunNow paused the task: %v", repo.statuses)
	}
	if repo.nextRunWrites != 0 {
		t.Fatal("RunNow rescheduled the task")
	}
}
