// Package liveevents publishes task, execution and approval state changes to
// the WebSocket hub.
//
// It decorates the repositories rather than the services because status moves
// happen from a dozen places (task launch, specialist boards, the task run
// service, chat tools); the repository write is the one choke point they all
// share. Publishing is best-effort: a failed lookup or a full hub drops the
// event, never the write. Clients treat events as "refetch now" hints and keep
// polling as the source of truth, so a dropped event costs latency only.
package liveevents

import (
	"context"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	ws "github.com/jobshout/server/internal/websocket"
)

// Event types added for live work. The task and agent ones live in the
// websocket package already.
const (
	EventExecutionStatusChanged = "execution.status_changed"
	EventApprovalRequested      = "approval.requested"
	EventApprovalDecided        = "approval.decided"
)

// ExecutionStatusChangedPayload is the payload for EventExecutionStatusChanged.
type ExecutionStatusChangedPayload struct {
	ExecutionID string `json:"execution_id"`
	AgentID     string `json:"agent_id"`
	Status      string `json:"status"`
}

// ApprovalPayload is the payload for EventApprovalRequested and
// EventApprovalDecided. Tool input is deliberately left out: it can carry
// arguments the viewer of a push or a socket frame should not see, and the
// client fetches the approval by id anyway.
type ApprovalPayload struct {
	ApprovalID  string `json:"approval_id"`
	ExecutionID string `json:"execution_id"`
	AgentID     string `json:"agent_id"`
	ToolName    string `json:"tool_name"`
	Status      string `json:"status"`
}

// Broadcaster is the part of the hub the decorators need.
type Broadcaster interface {
	BroadcastToOrg(orgID string, event ws.Event)
}

type publisher struct {
	out    Broadcaster
	logger *zap.Logger
}

func (p publisher) publish(orgID uuid.UUID, eventType string, payload any) {
	if orgID == uuid.Nil {
		return
	}
	ev, err := ws.NewEvent(eventType, orgID.String(), payload)
	if err != nil {
		p.logger.Warn("liveevents: encode event", zap.String("type", eventType), zap.Error(err))
		return
	}
	p.out.BroadcastToOrg(orgID.String(), ev)
}

// Tasks wraps a TaskRepository and publishes task.transitioned after every
// status change.
type Tasks struct {
	repository.TaskRepository
	projects repository.ProjectRepository
	pub      publisher
}

// NewTasks decorates repo.
func NewTasks(repo repository.TaskRepository, projects repository.ProjectRepository, out Broadcaster, logger *zap.Logger) *Tasks {
	return &Tasks{TaskRepository: repo, projects: projects, pub: publisher{out: out, logger: logger}}
}

func (t *Tasks) TransitionStatus(ctx context.Context, id uuid.UUID, status string, changedBy *uuid.UUID) error {
	before, _ := t.TaskRepository.FindByID(ctx, id)
	if err := t.TaskRepository.TransitionStatus(ctx, id, status, changedBy); err != nil {
		return err
	}
	t.published(ctx, before, status)
	return nil
}

func (t *Tasks) Reorder(ctx context.Context, id uuid.UUID, status string, position int, changedBy *uuid.UUID) error {
	before, _ := t.TaskRepository.FindByID(ctx, id)
	if err := t.TaskRepository.Reorder(ctx, id, status, position, changedBy); err != nil {
		return err
	}
	if before != nil && before.Status != status {
		t.published(ctx, before, status)
	}
	return nil
}

func (t *Tasks) published(ctx context.Context, before *model.Task, status string) {
	if before == nil {
		return
	}
	project, err := t.projects.FindByID(ctx, before.ProjectID)
	if err != nil || project == nil {
		return
	}
	t.pub.publish(project.OrgID, ws.EventTaskTransitioned, ws.TaskTransitionedPayload{
		TaskID:    before.ID.String(),
		TaskTitle: before.Title,
		OldStatus: before.Status,
		NewStatus: status,
		ProjectID: before.ProjectID.String(),
	})
}

// Executions wraps an ExecutionRepository and publishes
// execution.status_changed on start, completion, failure and cancellation.
type Executions struct {
	repository.ExecutionRepository
	pub publisher
}

// NewExecutions decorates repo.
func NewExecutions(repo repository.ExecutionRepository, out Broadcaster, logger *zap.Logger) *Executions {
	return &Executions{ExecutionRepository: repo, pub: publisher{out: out, logger: logger}}
}

func (e *Executions) MarkStarted(ctx context.Context, id uuid.UUID) error {
	return e.after(ctx, id, e.ExecutionRepository.MarkStarted(ctx, id))
}

func (e *Executions) MarkCompleted(ctx context.Context, id uuid.UUID, output string, totalTokens int, iterations int) error {
	return e.after(ctx, id, e.ExecutionRepository.MarkCompleted(ctx, id, output, totalTokens, iterations))
}

func (e *Executions) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, totalTokens int, iterations int) error {
	return e.after(ctx, id, e.ExecutionRepository.MarkFailed(ctx, id, errMsg, totalTokens, iterations))
}

func (e *Executions) MarkCancelled(ctx context.Context, id uuid.UUID) error {
	return e.after(ctx, id, e.ExecutionRepository.MarkCancelled(ctx, id))
}

func (e *Executions) after(ctx context.Context, id uuid.UUID, err error) error {
	if err != nil {
		return err
	}
	// Re-read rather than trust the method name: the row's status is what the
	// client will see when it refetches.
	exec, lookupErr := e.ExecutionRepository.GetByID(ctx, id)
	if lookupErr != nil || exec == nil {
		return nil
	}
	e.pub.publish(exec.OrgID, EventExecutionStatusChanged, ExecutionStatusChangedPayload{
		ExecutionID: exec.ID.String(),
		AgentID:     exec.AgentID.String(),
		Status:      exec.Status,
	})
	return nil
}

// Approvals wraps an ApprovalRepository and publishes approval.requested and
// approval.decided.
type Approvals struct {
	repository.ApprovalRepository
	pub publisher
}

// NewApprovals decorates repo.
func NewApprovals(repo repository.ApprovalRepository, out Broadcaster, logger *zap.Logger) *Approvals {
	return &Approvals{ApprovalRepository: repo, pub: publisher{out: out, logger: logger}}
}

func (a *Approvals) Create(ctx context.Context, ap *model.Approval) error {
	if err := a.ApprovalRepository.Create(ctx, ap); err != nil {
		return err
	}
	a.pub.publish(ap.OrgID, EventApprovalRequested, approvalPayload(ap))
	return nil
}

func (a *Approvals) UpdateDecision(ctx context.Context, id uuid.UUID, status string, reason string, decidedBy uuid.UUID) error {
	if err := a.ApprovalRepository.UpdateDecision(ctx, id, status, reason, decidedBy); err != nil {
		return err
	}
	if ap, err := a.ApprovalRepository.FindByID(ctx, id); err == nil && ap != nil {
		a.pub.publish(ap.OrgID, EventApprovalDecided, approvalPayload(ap))
	}
	return nil
}

func approvalPayload(ap *model.Approval) ApprovalPayload {
	return ApprovalPayload{
		ApprovalID:  ap.ID.String(),
		ExecutionID: ap.ExecutionID.String(),
		AgentID:     ap.AgentID.String(),
		ToolName:    ap.ToolName,
		Status:      ap.Status,
	}
}
