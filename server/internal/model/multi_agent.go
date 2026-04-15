package model

import (
	"time"

	"github.com/google/uuid"
)

// Multi-agent job status constants.
const (
	MultiAgentStatusPending   = "pending"
	MultiAgentStatusPlanning  = "planning"
	MultiAgentStatusExecuting = "executing"
	MultiAgentStatusReviewing = "reviewing"
	MultiAgentStatusCompleted = "completed"
	MultiAgentStatusFailed    = "failed"
)

// MultiAgentJob represents a collaborative task executed by a planner,
// executor, and reviewer agent working together.
type MultiAgentJob struct {
	ID           uuid.UUID  `json:"id"`
	OrgID        uuid.UUID  `json:"org_id"`
	TaskPrompt   string     `json:"task_prompt"`
	PlannerID    uuid.UUID  `json:"planner_id"`
	ExecutorID   uuid.UUID  `json:"executor_id"`
	ReviewerID   uuid.UUID  `json:"reviewer_id"`
	Status       string     `json:"status"`
	PlanOutput   *string    `json:"plan_output,omitempty"`
	ExecOutput   *string    `json:"exec_output,omitempty"`
	ReviewOutput *string    `json:"review_output,omitempty"`
	Approved     *bool      `json:"approved,omitempty"`
	Iterations   int        `json:"iterations"`
	MaxReview    int        `json:"max_review"`
	ErrorMsg     *string    `json:"error_msg,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
}

// RunMultiAgentRequest is the API payload for starting a multi-agent job.
type RunMultiAgentRequest struct {
	TaskPrompt string    `json:"task_prompt" validate:"required,min=3"`
	PlannerID  uuid.UUID `json:"planner_id" validate:"required"`
	ExecutorID uuid.UUID `json:"executor_id" validate:"required"`
	ReviewerID uuid.UUID `json:"reviewer_id" validate:"required"`
	MaxReview  int       `json:"max_review,omitempty"`
}
