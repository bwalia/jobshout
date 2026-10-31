package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Meta Lead Analyst pulls Instagram/Facebook Lead Ads Instant Form
// submissions via Graph API and runs LLM analysis (per-lead briefs + rollup).
const (
	BuiltinMetaLead   = "meta_lead_analyst"
	AgentNameMetaLead = "Meta Lead Analyst"
)

// Meta lead / run statuses.
const (
	MetaLeadStatusPending    = "pending"
	MetaLeadStatusAnalysing  = "analysing"
	MetaLeadStatusAnalysed   = "analysed"
	MetaLeadStatusFailed     = "failed"
	MetaLeadRunStatusRunning = "running"
	MetaLeadRunStatusDone    = "completed"
	MetaLeadRunStatusFailed  = "failed"
)

// MetaLeadConnection is a named Page connection for an org. The access token
// is never part of the JSON.
type MetaLeadConnection struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	Name        string     `json:"name"`
	PageID      string     `json:"page_id"`
	PageName    string     `json:"page_name"`
	ConnectedBy *uuid.UUID `json:"connected_by,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// MetaLeadRun is one launch or webhook batch of lead analysis.
type MetaLeadRun struct {
	ID           uuid.UUID       `json:"id"`
	OrgID        uuid.UUID       `json:"org_id"`
	TaskID       *uuid.UUID      `json:"task_id,omitempty"`
	ConnectionID uuid.UUID       `json:"connection_id"`
	FormID       string          `json:"form_id,omitempty"`
	FormName     string          `json:"form_name,omitempty"`
	Focus        string          `json:"focus,omitempty"`
	Status       string          `json:"status"`
	LeadCount    int             `json:"lead_count"`
	Rollup       json.RawMessage `json:"rollup,omitempty"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Source       string          `json:"source"` // launch | webhook
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// MetaLead is one Graph leadgen record plus optional LLM analysis.
type MetaLead struct {
	ID           uuid.UUID       `json:"id"`
	OrgID        uuid.UUID       `json:"org_id"`
	ConnectionID uuid.UUID       `json:"connection_id"`
	RunID        *uuid.UUID      `json:"run_id,omitempty"`
	LeadgenID    string          `json:"leadgen_id"`
	FormID       string          `json:"form_id,omitempty"`
	FieldData    json.RawMessage `json:"field_data,omitempty"`
	CreatedTime  *time.Time      `json:"created_time,omitempty"`
	Analysis     json.RawMessage `json:"analysis,omitempty"`
	Status       string          `json:"status"`
	ErrorMessage string          `json:"error_message,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

// MetaLeadBrief is the per-lead LLM analysis shape.
type MetaLeadBrief struct {
	Intent     string   `json:"intent"`
	FitScore   int      `json:"fit_score"`
	NextAction string   `json:"next_action"`
	SpamRisk   string   `json:"spam_risk"`
	Flags      []string `json:"flags"`
	Summary    string   `json:"summary"`
}

// MetaLeadRollup is the run-level LLM summary.
type MetaLeadRollup struct {
	Themes                 []string `json:"themes"`
	QualityMix             string   `json:"quality_mix"`
	RecommendedFollowups   []string `json:"recommended_followups"`
	LeadCount              int      `json:"lead_count"`
}
