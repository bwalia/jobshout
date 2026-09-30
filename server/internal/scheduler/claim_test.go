package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// sharedRepo is one scheduled_tasks row seen by several replicas' runners.
type sharedRepo struct {
	repository.SchedulerRepository
	mu   sync.Mutex
	task model.ScheduledTask
	runs int
}

func (s *sharedRepo) ListDueTasks(context.Context) ([]model.ScheduledTask, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return []model.ScheduledTask{s.task}, nil
}
func (s *sharedRepo) ClaimTask(_ context.Context, _ uuid.UUID, due *time.Time, next time.Time) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.task.NextRunAt == nil || due == nil || !s.task.NextRunAt.Equal(*due) {
		return false, nil
	}
	s.task.NextRunAt = &next
	return true, nil
}
func (s *sharedRepo) CreateRun(context.Context, *model.ScheduledTaskRun) error {
	s.mu.Lock()
	s.runs++
	s.mu.Unlock()
	return nil
}
func (s *sharedRepo) IncrementRunCount(context.Context, uuid.UUID) error { return nil }
func (s *sharedRepo) SetNextRunAt(context.Context, uuid.UUID, time.Time) error {
	return nil
}
func (s *sharedRepo) UpdateTask(context.Context, uuid.UUID, model.UpdateScheduledTaskRequest) (*model.ScheduledTask, error) {
	return &model.ScheduledTask{}, nil
}

// Every API replica runs a scheduler. When two of them list the same due task
// in the same tick, only one may dispatch it.
func TestTick_TwoReplicasDispatchADueTaskOnce(t *testing.T) {
	cron := "0 */5 * * *"
	due := time.Now().Add(-time.Minute).Truncate(time.Microsecond)
	repo := &sharedRepo{task: model.ScheduledTask{
		ID: uuid.New(), Name: "Trending articles", TaskType: "blog",
		ScheduleType: "cron", CronExpression: &cron, NextRunAt: &due,
	}}
	a := NewRunner(repo, nil, nil, nil, nil, zap.NewNop())
	b := NewRunner(repo, nil, nil, nil, nil, zap.NewNop())

	// Both replicas list the task before either claims it.
	tasksA, _ := repo.ListDueTasks(context.Background())
	tasksB, _ := repo.ListDueTasks(context.Background())
	wonA := a.claim(context.Background(), tasksA[0])
	wonB := b.claim(context.Background(), tasksB[0])

	if wonA == wonB {
		t.Fatalf("claims = %v/%v, want exactly one replica to win", wonA, wonB)
	}
}
