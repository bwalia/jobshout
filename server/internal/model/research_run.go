package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Research run statuses.
const (
	ResearchRunQueued    = "queued"
	ResearchRunRunning   = "running"
	ResearchRunCompleted = "completed"
	ResearchRunFailed    = "failed"
)

// Research run sources — which surface asked for the work. Free text in the
// column so a new caller needs no migration, but the ones in use are named here
// so they are spelled the same way everywhere.
const (
	ResearchSourceTaskManager = "task_manager"
	ResearchSourceChat        = "chat"
	ResearchSourceMail        = "mail"
	ResearchSourceBlog        = "blog"
	ResearchSourceAPI         = "api"
)

// ResearchRun is one execution of the Research Agent.
//
// It exists so research leaves a trace. Without it the agent could not appear
// on the board, a run could not be polled, and the findings lived only in
// whichever response happened to carry them.
type ResearchRun struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	AgentID     *uuid.UUID `json:"agent_id,omitempty"`
	TaskID      *uuid.UUID `json:"task_id,omitempty"`
	RequestedBy *uuid.UUID `json:"requested_by,omitempty"`

	Source  string   `json:"source"`
	Topic   string   `json:"topic"`
	Context string   `json:"context,omitempty"`
	URLs    []string `json:"urls,omitempty"`

	Status string `json:"status"`
	// Phase is the live research phase (planning/searching/reading/verifying),
	// written as the run progresses so the board can say what the agent is
	// doing rather than only that it is busy.
	Phase string `json:"phase,omitempty"`

	// Brief is the serialised research.Brief. Kept as raw JSON on the model so
	// this package does not depend on internal/research — the brief's shape is
	// owned there, and a model that imported it would invert the dependency.
	Brief        json.RawMessage `json:"brief,omitempty"`
	Usable       bool            `json:"usable"`
	ErrorMessage *string         `json:"error_message,omitempty"`

	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// IsTerminal reports whether the run has finished, either way.
func (r *ResearchRun) IsTerminal() bool {
	return r != nil && (r.Status == ResearchRunCompleted || r.Status == ResearchRunFailed)
}

// CreateResearchRunRequest is the API shape for starting a run.
type CreateResearchRunRequest struct {
	Topic   string     `json:"topic"`
	Context string     `json:"context,omitempty"`
	URLs    []string   `json:"urls,omitempty"`
	TaskID  *uuid.UUID `json:"task_id,omitempty"`
	// Wait runs the research synchronously and returns the completed run. It
	// exists so the callers that predate the async API keep working while they
	// are migrated; new callers should poll instead.
	Wait bool `json:"wait,omitempty"`
}
