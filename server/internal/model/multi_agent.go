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

// Agent-board columns. These are a separate vocabulary from agents.status —
// status is what an agent *is*, activity is what it is *doing right now*.
//
// Every value here must have a matching column in the frontend's COLUMNS array
// (web/nextjs/app/(app)/agent-board/page.tsx). The board groups with
// `out[e.activity]?.push(e)`, so an activity with no column is silently
// dropped and the agent vanishes from the board.
const (
	ActivityIdle       = "idle"
	ActivityPlanning   = "planning"
	ActivityExecuting  = "executing"
	ActivityReviewing  = "reviewing"
	ActivityPublishing = "publishing"
	ActivityFailed     = "failed"
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

// AgentBoardEntry is one row on the agent board: an agent paired with the
// activity derived from its most recent multi_agent_jobs participation or
// blog_runs attribution, whichever is newer. The
// frontend groups these by Activity to render Kanban columns.
type AgentBoardEntry struct {
	AgentID   uuid.UUID `json:"agent_id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	AvatarURL *string   `json:"avatar_url,omitempty"`

	// Activity is the column the card belongs in — one of the Activity*
	// constants above.
	Activity string `json:"activity"`

	// CurrentJobID is the multi-agent job or blog run driving the activity
	// (nil when idle).
	CurrentJobID *uuid.UUID `json:"current_job_id,omitempty"`

	// Role within the current work — planner | executor | reviewer for a
	// collaboration job, writer for a blog run (omitted when idle).
	JobRole *string `json:"job_role,omitempty"`

	// CurrentJobPrompt describes what the agent is doing: the task prompt for a
	// collaboration job, or the label of the running step for a blog run.
	CurrentJobPrompt *string `json:"current_job_prompt,omitempty"`

	// LastActiveAt is the most recent in-flight or terminal job timestamp,
	// for sorting columns by recency.
	LastActiveAt *time.Time `json:"last_active_at,omitempty"`
}
