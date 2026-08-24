package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

type ReviewRun struct {
	ID           uuid.UUID       `json:"id"`
	OrgID        uuid.UUID       `json:"org_id"`
	AgentID      *uuid.UUID      `json:"agent_id,omitempty"`
	RequestedBy  *uuid.UUID      `json:"requested_by,omitempty"`
	Repo         string          `json:"repo"`
	PRNumber     int             `json:"pr_number"`
	DryRun       bool            `json:"dry_run"`
	Force        bool            `json:"force"`
	Status       string          `json:"status"` // queued, running, completed, failed
	RemoteJobID  *string         `json:"remote_job_id,omitempty"`
	HeadSHA      *string         `json:"head_sha,omitempty"`
	Decision     *string         `json:"decision,omitempty"`
	Verdict      *string         `json:"verdict,omitempty"`
	Summary      *string         `json:"summary,omitempty"`
	GitHubURL    *string         `json:"github_url,omitempty"`
	Result       json.RawMessage `json:"result,omitempty"`
	StageLog     []string        `json:"stage_log,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	PollAttempts int             `json:"poll_attempts"`
	NextPollAt   *time.Time      `json:"next_poll_at,omitempty"`
	StartedAt    *time.Time      `json:"started_at,omitempty"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at"`
}

type CreateReviewRunRequest struct {
	Repo     string     `json:"repo" validate:"required"`
	PRNumber int        `json:"pr_number" validate:"required,min=1"`
	DryRun   *bool      `json:"dry_run"`
	Force    bool       `json:"force"`
	AgentID  *uuid.UUID `json:"agent_id"`
}
