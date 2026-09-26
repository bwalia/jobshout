package model

import (
	"time"

	"github.com/google/uuid"
)

// TaskRun is one on-demand execution of a board task by an agent, launched from
// the Task Manager. It records the exact configuration the run used so the run
// is reproducible and auditable even after the task itself changes. Heavy
// execution telemetry (tool calls, per-iteration detail) is on the linked
// AgentExecution via ExecutionID; the summary fields here mirror it so a runs
// list renders without a join.
type TaskRun struct {
	ID            uuid.UUID      `json:"id"`
	TaskID        uuid.UUID      `json:"task_id"`
	AgentID       uuid.UUID      `json:"agent_id"`
	OrgID         uuid.UUID      `json:"org_id"`
	ExecutionID   *uuid.UUID     `json:"execution_id"`
	Status        string         `json:"status"` // queued|running|completed|failed
	Prompt        string         `json:"prompt"`
	Engine        *string        `json:"engine"`
	ModelProvider *string        `json:"model_provider"`
	ModelName     *string        `json:"model_name"`
	SkillSlugs    []string       `json:"skill_slugs"`
	Inputs        map[string]any `json:"inputs"`
	Debug         bool           `json:"debug"`
	Output        *string        `json:"output"`
	ErrorMessage  *string        `json:"error_message"`
	TotalTokens   int            `json:"total_tokens"`
	CostUSD       float64        `json:"cost_usd"`
	LatencyMs     int            `json:"latency_ms"`
	Iterations    int            `json:"iterations"`
	RequestedBy   *uuid.UUID     `json:"requested_by"`
	StartedAt     *time.Time     `json:"started_at"`
	CompletedAt   *time.Time     `json:"completed_at"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
}

// TaskRun status values (mirror the CHECK constraint in migration 000027).
const (
	TaskRunStatusQueued    = "queued"
	TaskRunStatusRunning   = "running"
	TaskRunStatusCompleted = "completed"
	TaskRunStatusFailed    = "failed"
)

// CreateTaskRunRequest is the body of POST /api/v1/tasks/{taskID}/run. Every
// field is an override: with an empty body the run uses the task's assigned
// agent, the task's title+description as the prompt, and the agent's own model,
// engine and enabled skills.
type CreateTaskRunRequest struct {
	// AgentID overrides which agent runs the task. When nil the task's
	// assigned_agent_id is used; if the task has no assigned agent, AgentID is
	// required and the request is rejected without it.
	AgentID *uuid.UUID `json:"agent_id"`
	// Prompt fully replaces the derived prompt (title + description). When set,
	// the task's own text is not used. Inputs are still appended.
	Prompt *string `json:"prompt"`
	// Engine overrides the execution engine for this run only.
	Engine *string `json:"engine" validate:"omitempty,oneof=go_native langchain langgraph"`
	// ModelProvider / ModelName override the agent's model for this run only,
	// without mutating the agent record.
	ModelProvider *string `json:"model_provider"`
	ModelName     *string `json:"model_name"`
	// SkillSlugs are extra skills to load for this run, on top of whatever the
	// agent already has enabled. Resolved by slug against the org's skills (and
	// built-in skills). Unknown slugs are ignored, never fatal.
	SkillSlugs []string `json:"skill_slugs"`
	// Inputs are free-form key/value pairs appended to the prompt as context.
	Inputs map[string]any `json:"inputs"`
	// Debug requests the full engine trace be surfaced for this run.
	Debug bool `json:"debug"`
}
