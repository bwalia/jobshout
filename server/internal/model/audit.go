package model

import (
	"time"

	"github.com/google/uuid"
)

// AuditLog records a governance-relevant action.
type AuditLog struct {
	ID         uuid.UUID      `json:"id"`
	OrgID      uuid.UUID      `json:"org_id"`
	UserID     *uuid.UUID     `json:"user_id"`
	Action     string         `json:"action"`
	Resource   string         `json:"resource"`
	ResourceID *uuid.UUID     `json:"resource_id"`
	CostUSD    *float64       `json:"cost_usd"`
	OldValue   map[string]any `json:"old_value,omitempty"`
	NewValue   map[string]any `json:"new_value,omitempty"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	IPAddress  *string        `json:"ip_address"`
	CreatedAt  time.Time      `json:"created_at"`
}

// UsageRecord is a single granular usage event (partitioned table).
type UsageRecord struct {
	ID          uuid.UUID      `json:"id"`
	OrgID       uuid.UUID      `json:"org_id"`
	AgentID     *uuid.UUID     `json:"agent_id"`
	ExecutionID *uuid.UUID     `json:"execution_id"`
	TaskID      *uuid.UUID     `json:"task_id"`
	UserID      *uuid.UUID     `json:"user_id"`
	Provider    string         `json:"provider"`
	Model       string         `json:"model"`
	TokensIn    int            `json:"tokens_in"`
	TokensOut   int            `json:"tokens_out"`
	LatencyMs   int            `json:"latency_ms"`
	CostUSD     float64        `json:"cost_usd"`
	ToolCalls   int            `json:"tool_calls"`
	Retries     int            `json:"retries"`
	IsError     bool           `json:"is_error"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}

// CostRecord provides task-level cost attribution with breakdown.
type CostRecord struct {
	ID             uuid.UUID      `json:"id"`
	OrgID          uuid.UUID      `json:"org_id"`
	ExecutionID    *uuid.UUID     `json:"execution_id"`
	TaskID         *uuid.UUID     `json:"task_id"`
	AgentID        *uuid.UUID     `json:"agent_id"`
	CostType       string         `json:"cost_type"`
	LLMCostUSD     float64        `json:"llm_cost_usd"`
	ToolCostUSD    float64        `json:"tool_cost_usd"`
	ComputeCostUSD float64        `json:"compute_cost_usd"`
	TotalCostUSD   float64        `json:"total_cost_usd"`
	Provider       string         `json:"provider"`
	Model          string         `json:"model"`
	Breakdown      map[string]any `json:"breakdown"`
	CreatedAt      time.Time      `json:"created_at"`
}

// AgentScore holds the computed performance score for agent selection.
type AgentScore struct {
	ID           uuid.UUID `json:"id"`
	OrgID        uuid.UUID `json:"org_id"`
	AgentID      uuid.UUID `json:"agent_id"`
	TaskType     string    `json:"task_type"`
	SuccessRate  float64   `json:"success_rate"`
	AvgLatencyMs int       `json:"avg_latency_ms"`
	AvgCostUSD   float64   `json:"avg_cost_usd"`
	TotalRuns    int       `json:"total_runs"`
	Score        float64   `json:"score"`
	LastUpdated  time.Time `json:"last_updated"`
}

// PricingConfig holds a versioned pricing entry (system-wide or tenant-specific).
type PricingConfig struct {
	ID                   uuid.UUID      `json:"id"`
	OrgID                *uuid.UUID     `json:"org_id"`
	Provider             string         `json:"provider"`
	Model                string         `json:"model"`
	InputPricePerMToken  float64        `json:"input_price_per_m_token"`
	OutputPricePerMToken float64        `json:"output_price_per_m_token"`
	ComputePricePerSec   float64        `json:"compute_price_per_sec"`
	Version              int            `json:"version"`
	EffectiveFrom        time.Time      `json:"effective_from"`
	EffectiveUntil       *time.Time     `json:"effective_until"`
	IsActive             bool           `json:"is_active"`
	Metadata             map[string]any `json:"metadata,omitempty"`
	CreatedAt            time.Time      `json:"created_at"`
}

// ─── Requests ───────────────────────────────────────────────────────────────

// CreatePricingConfigRequest creates a pricing entry.
type CreatePricingConfigRequest struct {
	Provider             string   `json:"provider" validate:"required"`
	Model                string   `json:"model" validate:"required"`
	InputPricePerMToken  float64  `json:"input_price_per_m_token" validate:"gte=0"`
	OutputPricePerMToken float64  `json:"output_price_per_m_token" validate:"gte=0"`
	ComputePricePerSec   float64  `json:"compute_price_per_sec" validate:"gte=0"`
	EffectiveFrom        *string  `json:"effective_from"`
}

// AuditQueryParams filters audit log queries.
type AuditQueryParams struct {
	Action   string    `json:"action"`
	Resource string    `json:"resource"`
	UserID   *uuid.UUID `json:"user_id"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Limit    int       `json:"limit"`
}
