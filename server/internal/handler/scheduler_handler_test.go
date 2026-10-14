package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// fakeSchedulerRepo keeps tasks by id and records what an update asked for.
type fakeSchedulerRepo struct {
	byID      map[uuid.UUID]*model.ScheduledTask
	lastReq   *model.UpdateScheduledTaskRequest
	nextRunAt *time.Time
	updateErr error
	deleted   int
}

func (f *fakeSchedulerRepo) CreateTask(context.Context, *model.ScheduledTask) error { return nil }
func (f *fakeSchedulerRepo) GetTask(_ context.Context, id uuid.UUID) (*model.ScheduledTask, error) {
	t, ok := f.byID[id]
	if !ok {
		return nil, errors.New("not found")
	}
	cp := *t
	return &cp, nil
}
func (f *fakeSchedulerRepo) ListTasks(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.ScheduledTask], error) {
	return nil, nil
}
func (f *fakeSchedulerRepo) UpdateTask(_ context.Context, id uuid.UUID, req model.UpdateScheduledTaskRequest) (*model.ScheduledTask, error) {
	f.lastReq = &req
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	cp := *f.byID[id]
	return &cp, nil
}
func (f *fakeSchedulerRepo) DeleteTask(context.Context, uuid.UUID) error {
	f.deleted++
	return nil
}
func (f *fakeSchedulerRepo) ListDueTasks(context.Context) ([]model.ScheduledTask, error) {
	return nil, nil
}
func (f *fakeSchedulerRepo) IncrementRunCount(context.Context, uuid.UUID) error { return nil }
func (f *fakeSchedulerRepo) SetNextRunAt(_ context.Context, _ uuid.UUID, next time.Time) error {
	f.nextRunAt = &next
	return nil
}
func (f *fakeSchedulerRepo) ClaimTask(context.Context, uuid.UUID, *time.Time, time.Time) (bool, error) {
	return false, nil
}
func (f *fakeSchedulerRepo) CreateRun(context.Context, *model.ScheduledTaskRun) error { return nil }
func (f *fakeSchedulerRepo) ListRuns(context.Context, uuid.UUID, model.PaginationParams) (*model.PaginatedResponse[model.ScheduledTaskRun], error) {
	return &model.PaginatedResponse[model.ScheduledTaskRun]{}, nil
}

func TestSchedulerHandler_OrgIsolation(t *testing.T) {
	orgA, orgB, task := uuid.New(), uuid.New(), uuid.New()
	repo := &fakeSchedulerRepo{byID: map[uuid.UUID]*model.ScheduledTask{task: {ID: task, OrgID: orgB}}}
	h := NewSchedulerHandler(repo)
	params := map[string]string{"taskID": task.String()}

	for name, fn := range map[string]http.HandlerFunc{
		"get": h.GetByID, "update": h.Update, "delete": h.Delete, "runs": h.ListRuns,
	} {
		rec := httptest.NewRecorder()
		fn(rec, sprintReq(`{"name":"taken"}`, orgA, params))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s on another org's task = %d, want 404", name, rec.Code)
		}
	}
	if repo.lastReq != nil || repo.deleted != 0 {
		t.Fatal("another org's task was changed")
	}
}

func TestSchedulerHandler_UpdateSavesEveryField(t *testing.T) {
	org, task := uuid.New(), uuid.New()
	repo := &fakeSchedulerRepo{byID: map[uuid.UUID]*model.ScheduledTask{
		task: {ID: task, OrgID: org, TaskType: "workflow", ScheduleType: "once"},
	}}
	h := NewSchedulerHandler(repo)
	params := map[string]string{"taskID": task.String()}
	wf := uuid.NewString()

	rec := httptest.NewRecorder()
	h.Update(rec, sprintReq(`{"workflow_id":"`+wf+`","run_at":"2030-01-02T09:00:00Z",
		"retry_on_failure":true,"max_retries":4,"tags":["x"]}`, org, params))
	if rec.Code != http.StatusOK {
		t.Fatalf("update = %d %s", rec.Code, rec.Body.String())
	}
	got := repo.lastReq
	if got.WorkflowID == nil || *got.WorkflowID != wf || got.RunAt == nil ||
		got.RetryOnFailure == nil || !*got.RetryOnFailure || got.MaxRetries == nil || *got.MaxRetries != 4 ||
		len(got.Tags) != 1 {
		t.Fatalf("update request lost fields: %+v", got)
	}
	// A one-time task's new time is when it next runs.
	if want := time.Date(2030, 1, 2, 9, 0, 0, 0, time.UTC); repo.nextRunAt == nil || !repo.nextRunAt.Equal(want) {
		t.Fatalf("next_run_at = %v, want %v", repo.nextRunAt, want)
	}

	for body, want := range map[string]int{
		`{"workflow_id":"nope"}`:        http.StatusBadRequest,
		`{"run_at":"2030-01-02 09:00"}`: http.StatusBadRequest,
	} {
		rec = httptest.NewRecorder()
		h.Update(rec, sprintReq(body, org, params))
		if rec.Code != want {
			t.Errorf("%s = %d, want %d", body, rec.Code, want)
		}
	}

	// The repository refuses another org's workflow; that is the caller's
	// mistake, not a server error.
	repo.updateErr = repository.ErrScheduleTargetNotFound
	rec = httptest.NewRecorder()
	h.Update(rec, sprintReq(`{"workflow_id":"`+uuid.NewString()+`"}`, org, params))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("foreign workflow = %d, want 400", rec.Code)
	}
}
