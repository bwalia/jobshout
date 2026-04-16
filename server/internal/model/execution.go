package model

import (
	"time"

	"github.com/google/uuid"
)

// AgentExecution is a single invocation of an agent against a task prompt.
// It records the full lifecycle: input → iterations → output.
type AgentExecution struct {
	ID             uuid.UUID  `json:"id"`
	AgentID        uuid.UUID  `json:"agent_id"`
	OrgID          uuid.UUID  `json:"org_id"`
	WorkflowRunID  *uuid.UUID `json:"workflow_run_id"`
	StepID         *uuid.UUID `json:"step_id"`
	InputPrompt    string     `json:"input_prompt"`
	Output         *string    `json:"output"`
	Status         string     `json:"status"`
	ErrorMessage   *string    `json:"error_message"`
	TotalTokens    int        `json:"total_tokens"`
	InputTokens    int        `json:"input_tokens"`
	OutputTokens   int        `json:"output_tokens"`
	LatencyMs      int        `json:"latency_ms"`
	CostUSD        float64    `json:"cost_usd"`
	ModelName      *string    `json:"model_name"`
	ModelProvider  *string    `json:"model_provider"`
	Iterations     int        `json:"iterations"`
	EngineType     string     `json:"engine_type"`
	StartedAt      *time.Time `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	CreatedAt      time.Time  `json:"created_at"`
	// ToolCalls is populated on read — not stored inline.
	ToolCalls []ExecutionToolCall `json:"tool_calls,omitempty"`
}

// ExecutionToolCall records a single tool invocation during an execution.
type ExecutionToolCall struct {
	ID           uuid.UUID `json:"id"`
	ExecutionID  uuid.UUID `json:"execution_id"`
	ToolName     string    `json:"tool_name"`
	Input        map[string]any `json:"input"`
	Output       *string   `json:"output"`
	ErrorMessage *string   `json:"error_message"`
	DurationMs   int       `json:"duration_ms"`
	CalledAt     time.Time `json:"called_at"`
}

// Execution status values.
const (
	ExecutionStatusPending   = "pending"
	ExecutionStatusRunning   = "running"
	ExecutionStatusCompleted = "completed"
	ExecutionStatusFailed    = "failed"
)

// Workflow run status values.
const (
	WorkflowRunStatusPending   = "pending"
	WorkflowRunStatusRunning   = "running"
	WorkflowRunStatusCompleted = "completed"
	WorkflowRunStatusFailed    = "failed"
)

// --- request / response types ---

type ExecuteAgentRequest struct {
	// Prompt is the task description handed to the agent.
	Prompt string `json:"prompt" validate:"required,min=1"`
	// EngineOverride allows per-call engine selection without changing the agent record.
	EngineOverride *string `json:"engine_override,omitempty" validate:"omitempty,oneof=go_native langchain langgraph"`
}

// LangChainRunTrace records a single chain step during a LangChain execution.
type LangChainRunTrace struct {
	ID          uuid.UUID `json:"id"`
	ExecutionID uuid.UUID `json:"execution_id"`
	RunID       string    `json:"run_id"`
	ChainType   string    `json:"chain_type"`
	InputText   *string   `json:"input_text"`
	OutputText  *string   `json:"output_text"`
	Error       *string   `json:"error"`
	LatencyMs   int       `json:"latency_ms"`
	TotalTokens int       `json:"total_tokens"`
	CreatedAt   time.Time `json:"created_at"`
}

// LangGraphStateSnapshot captures graph state after each node execution.
type LangGraphStateSnapshot struct {
	ID          uuid.UUID      `json:"id"`
	ExecutionID uuid.UUID      `json:"execution_id"`
	StepNumber  int            `json:"step_number"`
	NodeName    string         `json:"node_name"`
	StateJSON   map[string]any `json:"state_json"`
	CreatedAt   time.Time      `json:"created_at"`
}
