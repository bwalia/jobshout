package model

import (
	"time"

	"github.com/google/uuid"
)

// BlogRunStatus values.
const (
	BlogRunStatusPending   = "pending"
	BlogRunStatusRunning   = "running"
	BlogRunStatusCompleted = "completed"
	BlogRunStatusFailed    = "failed"
)

// BlogRun records a single invocation of the auto-blog pipeline.
type BlogRun struct {
	ID           uuid.UUID        `json:"id"`
	OrgID        uuid.UUID        `json:"org_id"`
	TriggeredBy  *uuid.UUID       `json:"triggered_by"`
	Source       string           `json:"source"` // api | schedule
	Status       string           `json:"status"`
	Topics       []string         `json:"topics"`
	Model        *string          `json:"model"`
	Branch       *string          `json:"branch"`
	PRNumber     *int             `json:"pr_number"`
	PRURL        *string          `json:"pr_url"`
	Articles     []BlogRunArticle `json:"articles"`
	ErrorMessage *string          `json:"error_message"`
	StartedAt    *time.Time       `json:"started_at"`
	CompletedAt  *time.Time       `json:"completed_at"`
	CreatedAt    time.Time        `json:"created_at"`
}

// BlogRunArticle is the minimal per-article record persisted with the run —
// we deliberately don't store the full markdown (it lives in the PR).
type BlogRunArticle struct {
	Topic string `json:"topic"`
	Slug  string `json:"slug"`
	Path  string `json:"path"`
}

// GenerateBlogRequest is the HTTP request body for POST /api/v1/blogs/generate.
type GenerateBlogRequest struct {
	Topics      []string `json:"topics" validate:"required,min=1"`
	Model       string   `json:"model,omitempty"`
	MaxArticles int      `json:"max_articles,omitempty"`
}
