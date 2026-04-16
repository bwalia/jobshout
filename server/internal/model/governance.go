package model

import (
	"time"

	"github.com/google/uuid"
)

// ─── Budget ─────────────────────────────────────────────────────────────────

// OrgBudget defines spending limits for an organization within a billing period.
type OrgBudget struct {
	ID             uuid.UUID  `json:"id"`
	OrgID          uuid.UUID  `json:"org_id"`
	Period         string     `json:"period"`
	SoftLimitUSD   *float64   `json:"soft_limit_usd"`
	HardLimitUSD   *float64   `json:"hard_limit_usd"`
	AlertThreshold float64    `json:"alert_threshold"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// BudgetAlert records a budget threshold crossing event.
type BudgetAlert struct {
	ID          uuid.UUID `json:"id"`
	OrgID       uuid.UUID `json:"org_id"`
	BudgetID    uuid.UUID `json:"budget_id"`
	AlertType   string    `json:"alert_type"`
	SpendUSD    float64   `json:"spend_usd"`
	LimitUSD    float64   `json:"limit_usd"`
	TriggeredAt time.Time `json:"triggered_at"`
}

// Budget alert types.
const (
	AlertTypeThreshold = "threshold"
	AlertTypeSoftLimit = "soft_limit"
	AlertTypeHardLimit = "hard_limit"
)

// Budget periods.
const (
	BudgetPeriodDaily   = "daily"
	BudgetPeriodMonthly = "monthly"
	BudgetPeriodYearly  = "yearly"
)

// ─── Policy ─────────────────────────────────────────────────────────────────

// AgentPolicy defines governance rules for an agent (or org-wide default when AgentID is nil).
type AgentPolicy struct {
	ID               uuid.UUID  `json:"id"`
	OrgID            uuid.UUID  `json:"org_id"`
	AgentID          *uuid.UUID `json:"agent_id"`
	MaxTokensPerExec *int       `json:"max_tokens_per_exec"`
	AllowedModels    []string   `json:"allowed_models"`
	AllowedProviders []string   `json:"allowed_providers"`
	MaxCostPerExec   *float64   `json:"max_cost_per_exec"`
	MaxExecsPerDay   *int       `json:"max_execs_per_day"`
	MaxExecsPerHour  *int       `json:"max_execs_per_hour"`
	Enabled          bool       `json:"enabled"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// ─── Usage Rollups ──────────────────────────────────────────────────────────

// UsageRollup is a pre-aggregated usage record for analytics.
type UsageRollup struct {
	ID            uuid.UUID  `json:"id"`
	OrgID         uuid.UUID  `json:"org_id"`
	AgentID       *uuid.UUID `json:"agent_id"`
	ModelProvider string     `json:"model_provider"`
	ModelName     string     `json:"model_name"`
	PeriodType    string     `json:"period_type"`
	PeriodStart   time.Time  `json:"period_start"`
	ExecCount     int        `json:"exec_count"`
	InputTokens   int64      `json:"input_tokens"`
	OutputTokens  int64      `json:"output_tokens"`
	TotalTokens   int64      `json:"total_tokens"`
	CostUSD       float64    `json:"cost_usd"`
	AvgLatencyMs  int        `json:"avg_latency_ms"`
	ErrorCount    int        `json:"error_count"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// ─── Analytics View Structs ─────────────────────────────────────────────────

// OrgUsageSummary is an aggregated view of an organization's usage.
type OrgUsageSummary struct {
	OrgID          uuid.UUID           `json:"org_id"`
	TotalCostUSD   float64             `json:"total_cost_usd"`
	TotalExecs     int                 `json:"total_execs"`
	TotalInputTok  int64               `json:"total_input_tokens"`
	TotalOutputTok int64               `json:"total_output_tokens"`
	TotalErrors    int                 `json:"total_errors"`
	AvgLatencyMs   int                 `json:"avg_latency_ms"`
	ByProvider     []ProviderBreakdown `json:"by_provider"`
}

// ProviderBreakdown breaks down usage by LLM provider.
type ProviderBreakdown struct {
	Provider     string  `json:"provider"`
	CostUSD      float64 `json:"cost_usd"`
	ExecCount    int     `json:"exec_count"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
}

// AgentAnalytics is a per-agent analytics view.
type AgentAnalytics struct {
	AgentID      uuid.UUID `json:"agent_id"`
	AgentName    string    `json:"agent_name,omitempty"`
	TotalRuns    int       `json:"total_runs"`
	SuccessRate  float64   `json:"success_rate"`
	AvgLatencyMs int       `json:"avg_latency_ms"`
	TotalCostUSD float64   `json:"total_cost_usd"`
	TotalTokens  int64     `json:"total_tokens"`
	ErrorCount   int       `json:"error_count"`
}

// ─── Request / Response Types ───────────────────────────────────────────────

// CreateBudgetRequest is the payload for creating or updating a budget.
type CreateBudgetRequest struct {
	Period         string   `json:"period" validate:"required,oneof=daily monthly yearly"`
	SoftLimitUSD   *float64 `json:"soft_limit_usd" validate:"omitempty,gte=0"`
	HardLimitUSD   *float64 `json:"hard_limit_usd" validate:"omitempty,gte=0"`
	AlertThreshold *float64 `json:"alert_threshold" validate:"omitempty,gte=0,lte=1"`
	Enabled        *bool    `json:"enabled"`
}

// CreatePolicyRequest is the payload for creating or updating a policy.
type CreatePolicyRequest struct {
	AgentID          *uuid.UUID `json:"agent_id"`
	MaxTokensPerExec *int       `json:"max_tokens_per_exec" validate:"omitempty,gte=1"`
	AllowedModels    []string   `json:"allowed_models"`
	AllowedProviders []string   `json:"allowed_providers"`
	MaxCostPerExec   *float64   `json:"max_cost_per_exec" validate:"omitempty,gte=0"`
	MaxExecsPerDay   *int       `json:"max_execs_per_day" validate:"omitempty,gte=1"`
	MaxExecsPerHour  *int       `json:"max_execs_per_hour" validate:"omitempty,gte=1"`
	Enabled          *bool      `json:"enabled"`
}

// UsageQueryParams controls usage analytics queries.
type UsageQueryParams struct {
	AgentID     *uuid.UUID `json:"agent_id"`
	Provider    string     `json:"provider"`
	Model       string     `json:"model"`
	From        time.Time  `json:"from"`
	To          time.Time  `json:"to"`
	Granularity string     `json:"granularity"` // "hourly" or "daily"
}
