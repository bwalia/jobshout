package model

import (
	"time"

	"github.com/google/uuid"
)

// LLMProviderConfig is a user-managed LLM provider configuration stored in DB.
type LLMProviderConfig struct {
	ID           uuid.UUID      `json:"id"`
	OrgID        uuid.UUID      `json:"org_id"`
	Name         string         `json:"name"`
	ProviderType string         `json:"provider_type"` // ollama, openai, claude
	BaseURL      string         `json:"base_url"`
	APIKey       string         `json:"api_key,omitempty"` // masked in responses
	DefaultModel string         `json:"default_model"`
	IsDefault    bool           `json:"is_default"`
	IsActive     bool           `json:"is_active"`
	ConfigJSON   map[string]any `json:"config_json"`
	CreatedBy    *uuid.UUID     `json:"created_by"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

type CreateLLMProviderRequest struct {
	Name         string         `json:"name" validate:"required,min=2"`
	ProviderType string         `json:"provider_type" validate:"required,oneof=ollama openai claude"`
	BaseURL      string         `json:"base_url"`
	APIKey       string         `json:"api_key"`
	DefaultModel string         `json:"default_model" validate:"required"`
	IsDefault    bool           `json:"is_default"`
	ConfigJSON   map[string]any `json:"config_json"`
}

type UpdateLLMProviderRequest struct {
	Name         *string        `json:"name"`
	BaseURL      *string        `json:"base_url"`
	APIKey       *string        `json:"api_key"`
	DefaultModel *string        `json:"default_model"`
	IsDefault    *bool          `json:"is_default"`
	IsActive     *bool          `json:"is_active"`
	ConfigJSON   map[string]any `json:"config_json"`
}

// ScheduledTask is a recurring or one-time scheduled execution of an agent,
// workflow, or planner→executor→reviewer multi-agent collaboration.
type ScheduledTask struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	Name        string     `json:"name"`
	Description *string    `json:"description"`
	TaskType    string     `json:"task_type"` // agent | workflow | multi_agent | blog
	AgentID     *uuid.UUID `json:"agent_id"`
	WorkflowID  *uuid.UUID `json:"workflow_id"`
	// Multi-agent collaboration targets (used when task_type = "multi_agent").
	PlannerID        *uuid.UUID     `json:"planner_id,omitempty"`
	ExecutorID       *uuid.UUID     `json:"executor_id,omitempty"`
	ReviewerID       *uuid.UUID     `json:"reviewer_id,omitempty"`
	MaxReview        int            `json:"max_review,omitempty"`
	InputPrompt      string         `json:"input_prompt"`
	InputJSON        map[string]any `json:"input_json"`
	ProviderConfigID *uuid.UUID     `json:"provider_config_id"`
	ModelOverride    *string        `json:"model_override"`
	ScheduleType     string         `json:"schedule_type"` // cron | interval | once
	CronExpression   *string        `json:"cron_expression"`
	IntervalSeconds  *int           `json:"interval_seconds"`
	RunAt            *time.Time     `json:"run_at"`
	// SchedulePreset is what the user originally picked (e.g. "every_midnight").
	// We keep it so the UI can render the same friendly label rather than
	// reverse-engineering it from the cron expression.
	SchedulePreset *string    `json:"schedule_preset,omitempty"`
	Status         string     `json:"status"` // active | paused | completed | failed
	LastRunAt      *time.Time `json:"last_run_at"`
	NextRunAt      *time.Time `json:"next_run_at"`
	RunCount       int        `json:"run_count"`
	MaxRuns        *int       `json:"max_runs"`
	RetryOnFailure bool       `json:"retry_on_failure"`
	MaxRetries     int        `json:"max_retries"`
	Priority       string     `json:"priority"`
	Tags           []string   `json:"tags"`
	CreatedBy      *uuid.UUID `json:"created_by"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type CreateScheduledTaskRequest struct {
	Name        string  `json:"name" validate:"required,min=2"`
	Description *string `json:"description"`
	TaskType    string  `json:"task_type" validate:"required,oneof=agent workflow multi_agent blog"`
	AgentID     *string `json:"agent_id"`
	WorkflowID  *string `json:"workflow_id"`
	// Multi-agent fields (required when task_type = "multi_agent").
	PlannerID        *string        `json:"planner_id"`
	ExecutorID       *string        `json:"executor_id"`
	ReviewerID       *string        `json:"reviewer_id"`
	MaxReview        int            `json:"max_review"`
	InputPrompt      string         `json:"input_prompt"`
	InputJSON        map[string]any `json:"input_json"`
	ProviderConfigID *string        `json:"provider_config_id"`
	ModelOverride    *string        `json:"model_override"`
	// SchedulePreset, when set, takes precedence over CronExpression. The handler
	// translates presets (e.g. "every_midnight") to a cron expression so users
	// don't need to know cron syntax to schedule common cadences.
	SchedulePreset  *string  `json:"schedule_preset" validate:"omitempty,oneof=every_midnight every_morning_9am every_morning_10am hourly every_15m every_5m weekdays_9am weekly_monday_9am"`
	ScheduleType    string   `json:"schedule_type" validate:"required,oneof=cron interval once"`
	CronExpression  *string  `json:"cron_expression"`
	IntervalSeconds *int     `json:"interval_seconds"`
	RunAt           *string  `json:"run_at"`
	MaxRuns         *int     `json:"max_runs"`
	RetryOnFailure  bool     `json:"retry_on_failure"`
	MaxRetries      int      `json:"max_retries"`
	Priority        string   `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	Tags            []string `json:"tags"`
}

type UpdateScheduledTaskRequest struct {
	Name             *string        `json:"name"`
	Description      *string        `json:"description"`
	InputPrompt      *string        `json:"input_prompt"`
	InputJSON        map[string]any `json:"input_json"`
	ProviderConfigID *string        `json:"provider_config_id"`
	ModelOverride    *string        `json:"model_override"`
	CronExpression   *string        `json:"cron_expression"`
	IntervalSeconds  *int           `json:"interval_seconds"`
	RunAt            *string        `json:"run_at"`
	Status           *string        `json:"status" validate:"omitempty,oneof=active paused completed"`
	MaxRuns          *int           `json:"max_runs"`
	RetryOnFailure   *bool          `json:"retry_on_failure"`
	MaxRetries       *int           `json:"max_retries"`
	Priority         *string        `json:"priority" validate:"omitempty,oneof=low medium high critical"`
	Tags             []string       `json:"tags"`
}

// ScheduledTaskRun is a single execution of a scheduled task.
type ScheduledTaskRun struct {
	ID              uuid.UUID  `json:"id"`
	ScheduledTaskID uuid.UUID  `json:"scheduled_task_id"`
	ExecutionID     *uuid.UUID `json:"execution_id"`
	WorkflowRunID   *uuid.UUID `json:"workflow_run_id"`
	MultiAgentJobID *uuid.UUID `json:"multi_agent_job_id,omitempty"`
	Status          string     `json:"status"`
	Output          *string    `json:"output"`
	ErrorMessage    *string    `json:"error_message"`
	StartedAt       *time.Time `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at"`
	CreatedAt       time.Time  `json:"created_at"`
}
