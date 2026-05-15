package model

import (
	"time"

	"github.com/google/uuid"
)

// Sprint statuses match the migration check on sprints.status.
const (
	SprintStatusPlanning  = "planning"
	SprintStatusActive    = "active"
	SprintStatusCompleted = "completed"
	SprintStatusCancelled = "cancelled"
)

// Sprint is a time-boxed iteration grouping multi-agent jobs together.
type Sprint struct {
	ID        uuid.UUID  `json:"id"`
	OrgID     uuid.UUID  `json:"org_id"`
	Name      string     `json:"name"`
	Goal      *string    `json:"goal,omitempty"`
	Status    string     `json:"status"`
	StartAt   *time.Time `json:"start_at,omitempty"`
	EndAt     *time.Time `json:"end_at,omitempty"`
	Velocity  *float64   `json:"velocity,omitempty"`
	CreatedBy *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// SprintDetail is Sprint plus its joined jobs and assigned agents — used by
// the sprint-board page so the UI gets everything in one round-trip.
type SprintDetail struct {
	Sprint
	Jobs   []MultiAgentJob   `json:"jobs"`
	Agents []SprintAgentInfo `json:"agents"`
	// Stats are derived counts so the UI can render the sprint header without
	// re-iterating the job list.
	Stats SprintStats `json:"stats"`
}

type SprintAgentInfo struct {
	AgentID   uuid.UUID `json:"agent_id"`
	Name      string    `json:"name"`
	Role      string    `json:"role"`
	AvatarURL *string   `json:"avatar_url,omitempty"`
	RoleLabel string    `json:"role_label"` // planner | executor | reviewer | any
}

type SprintStats struct {
	TotalJobs     int `json:"total_jobs"`
	CompletedJobs int `json:"completed_jobs"`
	FailedJobs    int `json:"failed_jobs"`
	InFlightJobs  int `json:"in_flight_jobs"`
}

type CreateSprintRequest struct {
	Name    string     `json:"name" validate:"required,min=2"`
	Goal    *string    `json:"goal"`
	StartAt *time.Time `json:"start_at"`
	EndAt   *time.Time `json:"end_at"`
}

type UpdateSprintRequest struct {
	Name    *string    `json:"name"`
	Goal    *string    `json:"goal"`
	Status  *string    `json:"status" validate:"omitempty,oneof=planning active completed cancelled"`
	StartAt *time.Time `json:"start_at"`
	EndAt   *time.Time `json:"end_at"`
}

type AddSprintJobRequest struct {
	JobID    uuid.UUID `json:"job_id" validate:"required"`
	Position int       `json:"position"`
}

type AddSprintAgentRequest struct {
	AgentID   uuid.UUID `json:"agent_id" validate:"required"`
	RoleLabel string    `json:"role_label" validate:"omitempty,oneof=planner executor reviewer any"`
}
