package model

import (
	"time"

	"github.com/google/uuid"
)

// Approval status constants.
const (
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
)

// Approval records a gated agent tool call awaiting (or having received) a human
// decision. When an execution pauses on a gated tool, ResumeState holds the
// serialised executor state needed to continue the ReAct loop once a human
// approves or rejects.
type Approval struct {
	ID          uuid.UUID      `json:"id"`
	OrgID       uuid.UUID      `json:"org_id"`
	ExecutionID uuid.UUID      `json:"execution_id"`
	AgentID     uuid.UUID      `json:"agent_id"`
	ToolName    string         `json:"tool_name"`
	ToolInput   map[string]any `json:"tool_input"`
	Status      string         `json:"status"`
	Reason      *string        `json:"reason"`
	RequestedAt time.Time      `json:"requested_at"`
	DecidedBy   *uuid.UUID     `json:"decided_by"`
	DecidedAt   *time.Time     `json:"decided_at"`

	// ResumeState is the JSONB-serialised executor state used to resume the
	// paused run. It is never exposed over the API.
	ResumeState []byte `json:"-"`

	// DeciderName is a transient, best-effort display name for the human who
	// decided; it is never persisted. It is populated when resuming so a
	// rejection message fed back to the agent can name the reviewer.
	DeciderName string `json:"decided_by_name,omitempty"`
}

// DecideApprovalRequest is the body of POST /api/v1/approvals/{id}/decide.
type DecideApprovalRequest struct {
	Decision string `json:"decision" validate:"required,oneof=approve reject"`
	Reason   string `json:"reason"`
}
