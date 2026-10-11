package webhook

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// The event sources are hooked at the repository layer, where the state
// change is recorded, so every path that finishes a run or records an
// approval decision emits — including runs resumed after an approval —
// without the services knowing about webhooks. The decorators embed the
// repository interface, so methods added to it later pass straight through.

// maxOutput caps the run output carried in a webhook (the full output stays
// on GET /api/v1/task-runs/{id}).
const maxOutput = 64 << 10

// RunData is the data of run.completed / run.failed.
type RunData struct {
	RunID           uuid.UUID         `json:"run_id"`
	TaskID          uuid.UUID         `json:"task_id"`
	AgentID         uuid.UUID         `json:"agent_id"`
	ExecutionID     *uuid.UUID        `json:"execution_id"`
	Status          string            `json:"status"`
	Output          *string           `json:"output"`
	OutputTruncated bool              `json:"output_truncated,omitempty"`
	ErrorMessage    *string           `json:"error_message"`
	Context         *model.RunContext `json:"context,omitempty"`
	SkillSlugs      []string          `json:"skill_slugs"`
	Inputs          map[string]any    `json:"inputs,omitempty"`
	// ModelProvider / ModelName are the run's requested model (the agent's
	// or the run's override); an org routing chain may have served it.
	ModelProvider *string `json:"model_provider,omitempty"`
	ModelName     *string `json:"model_name,omitempty"`
	// PromptVersion is the agent's metadata.prompt_version when it has one
	// (e.g. the Property PA agents), read when the event is emitted.
	PromptVersion string     `json:"prompt_version,omitempty"`
	TotalTokens   int        `json:"total_tokens"`
	CostUSD       float64    `json:"cost_usd"`
	LatencyMs     int        `json:"latency_ms"`
	StartedAt     *time.Time `json:"started_at"`
	CompletedAt   *time.Time `json:"completed_at"`
}

func runData(run *model.TaskRun) RunData {
	d := RunData{
		RunID: run.ID, TaskID: run.TaskID, AgentID: run.AgentID, ExecutionID: run.ExecutionID,
		Status: run.Status, Output: run.Output, ErrorMessage: run.ErrorMessage, Context: run.Context,
		SkillSlugs: run.SkillSlugs, Inputs: run.Inputs, ModelProvider: run.ModelProvider, ModelName: run.ModelName,
		TotalTokens: run.TotalTokens,
		CostUSD:     run.CostUSD, LatencyMs: run.LatencyMs, StartedAt: run.StartedAt, CompletedAt: run.CompletedAt,
	}
	if d.Output != nil && len(*d.Output) > maxOutput {
		cut := (*d.Output)[:maxOutput]
		d.Output, d.OutputTruncated = &cut, true
	}
	return d
}

// TaskRuns decorates a TaskRunRepository: an Update that moves a run into
// completed or failed emits run.completed / run.failed.
type TaskRuns struct {
	repository.TaskRunRepository
	Emitter Emitter
	// Agents, optional, resolves the agent's prompt_version for run events.
	Agents AgentFinder
}

// AgentFinder looks up an agent (repository.AgentRepository).
type AgentFinder interface {
	FindByID(ctx context.Context, id uuid.UUID) (*model.Agent, error)
}

func (t *TaskRuns) runData(ctx context.Context, run *model.TaskRun) RunData {
	d := runData(run)
	if t.Agents != nil && run.AgentID != uuid.Nil {
		if a, err := t.Agents.FindByID(ctx, run.AgentID); err == nil && a != nil && a.OrgID == run.OrgID {
			d.PromptVersion, _ = a.Metadata["prompt_version"].(string)
		}
	}
	return d
}

// Update persists the run, then emits when its status became terminal.
func (t *TaskRuns) Update(ctx context.Context, run *model.TaskRun) error {
	prev := ""
	if before, err := t.TaskRunRepository.GetByID(ctx, run.ID); err == nil && before != nil {
		prev = before.Status
	}
	if err := t.TaskRunRepository.Update(ctx, run); err != nil {
		return err
	}
	if t.Emitter == nil || run.Status == prev {
		return nil
	}
	switch run.Status {
	case model.TaskRunStatusCompleted:
		t.Emitter.Emit(ctx, run.OrgID, model.WebhookEventRunCompleted, t.runData(ctx, run))
	case model.TaskRunStatusFailed:
		t.Emitter.Emit(ctx, run.OrgID, model.WebhookEventRunFailed, t.runData(ctx, run))
	}
	return nil
}

// ApprovalData is the data of approval.requested / approval.decided.
type ApprovalData struct {
	ApprovalID  uuid.UUID      `json:"approval_id"`
	ExecutionID uuid.UUID      `json:"execution_id"`
	AgentID     uuid.UUID      `json:"agent_id"`
	TaskRunID   *uuid.UUID     `json:"task_run_id,omitempty"`
	ToolName    string         `json:"tool_name"`
	ToolInput   map[string]any `json:"tool_input"`
	Status      string         `json:"status"`
	Reason      *string        `json:"reason,omitempty"`
	RequestedAt time.Time      `json:"requested_at"`
	DecidedBy   *uuid.UUID     `json:"decided_by,omitempty"`
	DecidedAt   *time.Time     `json:"decided_at,omitempty"`
}

// runByExecution is implemented by task-run repositories that can map an
// execution back to its run (added by the approvals work); when present the
// approval events carry task_run_id.
type runByExecution interface {
	GetByExecutionID(ctx context.Context, executionID uuid.UUID) (*model.TaskRun, error)
}

// Approvals decorates an ApprovalRepository: Create emits approval.requested,
// a successful UpdateDecision emits approval.decided.
type Approvals struct {
	repository.ApprovalRepository
	Emitter Emitter
	// Runs, optional, resolves task_run_id.
	Runs any
}

func (a *Approvals) data(ctx context.Context, ap *model.Approval) ApprovalData {
	d := ApprovalData{
		ApprovalID: ap.ID, ExecutionID: ap.ExecutionID, AgentID: ap.AgentID, ToolName: ap.ToolName,
		ToolInput: ap.ToolInput, Status: ap.Status, Reason: ap.Reason, RequestedAt: ap.RequestedAt,
		DecidedBy: ap.DecidedBy, DecidedAt: ap.DecidedAt,
	}
	if rb, ok := a.Runs.(runByExecution); ok && ap.ExecutionID != uuid.Nil {
		if run, err := rb.GetByExecutionID(ctx, ap.ExecutionID); err == nil && run != nil {
			id := run.ID
			d.TaskRunID = &id
		}
	}
	return d
}

// Create records the approval, then emits approval.requested.
func (a *Approvals) Create(ctx context.Context, ap *model.Approval) error {
	if err := a.ApprovalRepository.Create(ctx, ap); err != nil {
		return err
	}
	if a.Emitter != nil {
		a.Emitter.Emit(ctx, ap.OrgID, model.WebhookEventApprovalRequested, a.data(ctx, ap))
	}
	return nil
}

// UpdateDecision records the decision, then emits approval.decided.
func (a *Approvals) UpdateDecision(ctx context.Context, id uuid.UUID, status string, reason string, decidedBy uuid.UUID) error {
	if err := a.ApprovalRepository.UpdateDecision(ctx, id, status, reason, decidedBy); err != nil {
		return err
	}
	if a.Emitter == nil {
		return nil
	}
	ap, err := a.ApprovalRepository.FindByID(ctx, id)
	if err != nil || ap == nil {
		return nil
	}
	a.Emitter.Emit(ctx, ap.OrgID, model.WebhookEventApprovalDecided, a.data(ctx, ap))
	return nil
}
