package model

import (
	"time"

	"github.com/google/uuid"
)

// Goal status constants for the Goal → Plan → Act → Observe → Reflect lifecycle.
const (
	GoalStatusPending    = "pending"
	GoalStatusPlanning   = "planning"
	GoalStatusExecuting  = "executing"
	GoalStatusReflecting = "reflecting"
	GoalStatusCompleted  = "completed"
	GoalStatusFailed     = "failed"
)

// AgentGoal represents a high-level objective assigned to an agent, executed
// via the autonomous Goal/Plan/Act/Observe/Reflect loop.
type AgentGoal struct {
	ID          uuid.UUID  `json:"id"`
	AgentID     uuid.UUID  `json:"agent_id"`
	OrgID       uuid.UUID  `json:"org_id"`
	SessionID   *uuid.UUID `json:"session_id,omitempty"`
	GoalText    string     `json:"goal_text"`
	Plan        []PlanStep `json:"plan"`
	Status      string     `json:"status"`
	Reflection  *string    `json:"reflection,omitempty"`
	Iterations  int        `json:"iterations"`
	MaxIter     int        `json:"max_iter"`
	ErrorMsg    *string    `json:"error_msg,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// PlanStep is a single step within the plan generated for a goal.
type PlanStep struct {
	Index       int    `json:"index"`
	Description string `json:"description"`
	ToolHint    string `json:"tool_hint,omitempty"`
	Completed   bool   `json:"completed"`
	Output      string `json:"output,omitempty"`
}

// CreateGoalRequest is the API payload for creating a new agent goal.
type CreateGoalRequest struct {
	GoalText  string     `json:"goal_text" validate:"required,min=3"`
	SessionID *uuid.UUID `json:"session_id,omitempty"`
	MaxIter   int        `json:"max_iter,omitempty"`
}
