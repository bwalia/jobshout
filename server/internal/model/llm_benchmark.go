package model

import (
	"time"

	"github.com/google/uuid"
)

// LLMCall is one recorded text LLM call (a usage_records row written by
// internal/llmbench). It holds metadata only — never prompts or replies.
type LLMCall struct {
	ID          uuid.UUID  `json:"id"`
	AgentID     *uuid.UUID `json:"agent_id,omitempty"`
	TaskID      *uuid.UUID `json:"task_id,omitempty"`
	TaskRunID   *uuid.UUID `json:"task_run_id,omitempty"`
	ExecutionID *uuid.UUID `json:"execution_id,omitempty"`
	RunKind     string     `json:"run_kind"`
	RunID       string     `json:"run_id"`
	Stage       string     `json:"stage"`
	Attempt     int        `json:"attempt"`
	// Provider is the concrete client that made the call. Model is the model
	// the provider's reply named when ModelReported, else the model sent;
	// RequestedModel is always the model sent.
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	ModelReported  bool   `json:"model_reported"`
	RequestedModel string `json:"requested_model"`
	// DurationMs is the whole call, retries included; APIDurationMs only the
	// HTTP exchanges with the provider (null when not measured).
	DurationMs    int  `json:"duration_ms"`
	APIDurationMs *int `json:"api_duration_ms"`
	// APIAttemptCount is how many HTTP requests were actually sent (null when
	// not reported); APIAttempts lists each one.
	APIAttemptCount *int             `json:"api_attempt_count"`
	APIAttempts     []LLMCallAttempt `json:"api_attempts"`
	// Token counts are null when the provider did not report them.
	// TotalTokens is the provider's own total, never computed.
	InputTokens       *int       `json:"input_tokens"`
	OutputTokens      *int       `json:"output_tokens"`
	TotalTokens       *int       `json:"total_tokens"`
	ReasoningTokens   *int       `json:"reasoning_tokens"`
	ProviderRequestID *string    `json:"provider_request_id"`
	Retries           int        `json:"retries"`
	Status            string     `json:"status"`
	Error             string     `json:"error,omitempty"`
	StartedAt         *time.Time `json:"started_at"`
	CompletedAt       *time.Time `json:"completed_at"`
}

// LLMCallAttempt is one HTTP request a call actually sent to its provider.
type LLMCallAttempt struct {
	StartedAt  time.Time `json:"started_at"`
	DurationMs int       `json:"duration_ms"`
	HTTPStatus int       `json:"http_status,omitempty"`
	RequestID  string    `json:"request_id,omitempty"`
	Error      string    `json:"error,omitempty"`
}

// LLMModelStat aggregates raw calls for one provider + exact model (and,
// when grouped by stage, one stage). Durations are per call, in ms.
type LLMModelStat struct {
	Provider        string   `json:"provider"`
	Model           string   `json:"model"`
	Stage           *string  `json:"stage,omitempty"`
	Calls           int      `json:"calls"`
	Runs            int      `json:"runs"`
	Successes       int      `json:"successes"`
	Failures        int      `json:"failures"`
	Cancelled       int      `json:"cancelled"`
	AvgDurationMs   float64  `json:"avg_duration_ms"`
	MinDurationMs   int      `json:"min_duration_ms"`
	MaxDurationMs   int      `json:"max_duration_ms"`
	P50DurationMs   float64  `json:"p50_duration_ms"`
	P95DurationMs   float64  `json:"p95_duration_ms"`
	InputTokens     *int64   `json:"input_tokens"`
	OutputTokens    *int64   `json:"output_tokens"`
	TotalTokens     *int64   `json:"total_tokens"`
	CallsWithUsage  int      `json:"calls_with_usage"`
	AvgInputTokens  *float64 `json:"avg_input_tokens"`
	AvgOutputTokens *float64 `json:"avg_output_tokens"`
	Retries         int      `json:"retries"`
	RetriedCalls    int      `json:"retried_calls"`
}

// LLMRunSummary groups the calls of one run. RunDurationMs is the run's own
// wall-clock time from its run table (course_runs, blog_runs) and is kept
// apart from the per-call durations; it is null when the run kind has no run
// table or the run has not finished.
type LLMRunSummary struct {
	RunKind       string     `json:"run_kind"`
	RunID         string     `json:"run_id"`
	AgentID       *uuid.UUID `json:"agent_id,omitempty"`
	AgentName     string     `json:"agent_name"`
	Models        []string   `json:"models"`
	CallCount     int        `json:"call_count"`
	Failures      int        `json:"failures"`
	Retries       int        `json:"retries"`
	LLMTimeMs     int64      `json:"llm_time_ms"`
	InputTokens   *int64     `json:"input_tokens"`
	OutputTokens  *int64     `json:"output_tokens"`
	FirstCallAt   *time.Time `json:"first_call_at"`
	LastCallAt    *time.Time `json:"last_call_at"`
	RunStatus     string     `json:"run_status"`
	RunDurationMs *int64     `json:"run_duration_ms"`
}

// LLMRunDetail is one run with every call it made, in order.
type LLMRunDetail struct {
	LLMRunSummary
	Calls []LLMCall `json:"calls"`
}

// LLMBenchmarkFilter narrows benchmark queries. OrgID is mandatory and
// always applied.
type LLMBenchmarkFilter struct {
	OrgID    uuid.UUID
	From     time.Time
	To       time.Time
	RunKind  string
	RunID    string
	Provider string
	Model    string
	Stage    string
	ByStage  bool
	// Links to the work that made the calls; nil means any.
	TaskID      *uuid.UUID
	TaskRunID   *uuid.UUID
	ExecutionID *uuid.UUID
}
