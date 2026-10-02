package liveevents

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	ws "github.com/jobshout/server/internal/websocket"
)

type sink struct{ events []ws.Event }

func (s *sink) BroadcastToOrg(orgID string, e ws.Event) {
	e.OrgID = orgID
	s.events = append(s.events, e)
}

type fakeTasks struct {
	repository.TaskRepository
	task *model.Task
	err  error
}

func (f *fakeTasks) FindByID(context.Context, uuid.UUID) (*model.Task, error) {
	cp := *f.task
	return &cp, nil
}
func (f *fakeTasks) TransitionStatus(_ context.Context, _ uuid.UUID, status string, _ *uuid.UUID) error {
	if f.err != nil {
		return f.err
	}
	f.task.Status = status
	return nil
}
func (f *fakeTasks) Reorder(_ context.Context, _ uuid.UUID, status string, _ int, _ *uuid.UUID) error {
	f.task.Status = status
	return nil
}

type fakeProjects struct {
	repository.ProjectRepository
	orgID uuid.UUID
}

func (f *fakeProjects) FindByID(context.Context, uuid.UUID) (*model.Project, error) {
	return &model.Project{OrgID: f.orgID}, nil
}

func TestTaskTransitionPublishesToProjectOrg(t *testing.T) {
	org := uuid.New()
	task := &model.Task{ID: uuid.New(), ProjectID: uuid.New(), Title: "Scan", Status: "todo"}
	out := &sink{}
	repo := NewTasks(&fakeTasks{task: task}, &fakeProjects{orgID: org}, out, zap.NewNop())

	if err := repo.TransitionStatus(context.Background(), task.ID, "in_progress", nil); err != nil {
		t.Fatal(err)
	}
	if len(out.events) != 1 {
		t.Fatalf("events = %d, want 1", len(out.events))
	}
	ev := out.events[0]
	if ev.Type != ws.EventTaskTransitioned || ev.OrgID != org.String() {
		t.Fatalf("unexpected event %+v", ev)
	}
	var p ws.TaskTransitionedPayload
	_ = json.Unmarshal(ev.Payload, &p)
	if p.OldStatus != "todo" || p.NewStatus != "in_progress" {
		t.Fatalf("payload %+v", p)
	}

	// A reorder within the same column is not a transition.
	if err := repo.Reorder(context.Background(), task.ID, "in_progress", 3, nil); err != nil {
		t.Fatal(err)
	}
	if len(out.events) != 1 {
		t.Fatal("same-column reorder must not publish")
	}
}

func TestFailedWritePublishesNothing(t *testing.T) {
	task := &model.Task{ID: uuid.New(), ProjectID: uuid.New(), Status: "todo"}
	out := &sink{}
	boom := errors.New("boom")
	repo := NewTasks(&fakeTasks{task: task, err: boom}, &fakeProjects{orgID: uuid.New()}, out, zap.NewNop())

	if err := repo.TransitionStatus(context.Background(), task.ID, "done", nil); !errors.Is(err, boom) {
		t.Fatalf("got %v", err)
	}
	if len(out.events) != 0 {
		t.Fatal("a failed write must not publish")
	}
}

type fakeApprovals struct {
	repository.ApprovalRepository
	row *model.Approval
}

func (f *fakeApprovals) Create(_ context.Context, a *model.Approval) error {
	cp := *a
	f.row = &cp
	return nil
}
func (f *fakeApprovals) UpdateDecision(_ context.Context, _ uuid.UUID, status, _ string, _ uuid.UUID) error {
	f.row.Status = status
	return nil
}
func (f *fakeApprovals) FindByID(context.Context, uuid.UUID) (*model.Approval, error) {
	cp := *f.row
	return &cp, nil
}

func TestApprovalEventsOmitToolInput(t *testing.T) {
	org := uuid.New()
	out := &sink{}
	repo := NewApprovals(&fakeApprovals{}, out, zap.NewNop())
	a := &model.Approval{
		ID: uuid.New(), OrgID: org, ExecutionID: uuid.New(), AgentID: uuid.New(),
		ToolName: "deploy", Status: model.ApprovalStatusPending,
		ToolInput: map[string]any{"secret_arg": "do-not-leak"},
	}
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatal(err)
	}
	if err := repo.UpdateDecision(context.Background(), a.ID, model.ApprovalStatusApproved, "", uuid.New()); err != nil {
		t.Fatal(err)
	}
	if len(out.events) != 2 || out.events[0].Type != EventApprovalRequested || out.events[1].Type != EventApprovalDecided {
		t.Fatalf("unexpected events %+v", out.events)
	}
	for _, ev := range out.events {
		if strings.Contains(string(ev.Payload), "do-not-leak") {
			t.Fatal("tool input leaked into the event payload")
		}
	}
}

type fakeExecutions struct {
	repository.ExecutionRepository
	row *model.AgentExecution
}

func (f *fakeExecutions) MarkCompleted(context.Context, uuid.UUID, string, int, int) error {
	f.row.Status = "completed"
	return nil
}
func (f *fakeExecutions) GetByID(context.Context, uuid.UUID) (*model.AgentExecution, error) {
	cp := *f.row
	return &cp, nil
}

func TestExecutionCompletionPublishesStoredStatus(t *testing.T) {
	org := uuid.New()
	out := &sink{}
	row := &model.AgentExecution{ID: uuid.New(), AgentID: uuid.New(), OrgID: org, Status: "running"}
	repo := NewExecutions(&fakeExecutions{row: row}, out, zap.NewNop())

	if err := repo.MarkCompleted(context.Background(), row.ID, "ok", 10, 1); err != nil {
		t.Fatal(err)
	}
	if len(out.events) != 1 || out.events[0].Type != EventExecutionStatusChanged || out.events[0].OrgID != org.String() {
		t.Fatalf("unexpected events %+v", out.events)
	}
	var p ExecutionStatusChangedPayload
	_ = json.Unmarshal(out.events[0].Payload, &p)
	if p.Status != "completed" {
		t.Fatalf("payload %+v", p)
	}
}
