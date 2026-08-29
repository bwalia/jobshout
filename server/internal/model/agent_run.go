package model

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// Agent run statuses.
//
// An agent run is a thin envelope over whatever the specialist writes: it is
// queued, it is running, and then it has finished one way or the other. The
// detailed progress lives in the specialist's own row, which ExternalRunID
// names.
const (
	AgentRunQueued    = "queued"
	AgentRunRunning   = "running"
	AgentRunCompleted = "completed"
	AgentRunFailed    = "failed"
)

// Agent run sources — which surface asked.
const (
	AgentRunSourceTaskManager = "task_manager"
	AgentRunSourceChat        = "chat"
	AgentRunSourceAPI         = "api"
	AgentRunSourceScheduler   = "scheduler"
)

// AgentRun is one execution of one agent, however it was started.
//
// It exists because "run agent X with inputs Y" was implemented three separate
// times — a client-side switch in the Task Manager, a server-side switch in the
// chat tools, and a builtin-unaware generic loop — which is why the same agent
// behaved differently depending on which surface launched it. This row is the
// single record all of them now write.
type AgentRun struct {
	ID          uuid.UUID  `json:"id"`
	OrgID       uuid.UUID  `json:"org_id"`
	AgentID     uuid.UUID  `json:"agent_id"`
	TaskID      *uuid.UUID `json:"task_id,omitempty"`
	RequestedBy *uuid.UUID `json:"requested_by,omitempty"`

	// Builtin is the platform marker the run was dispatched on, empty for a
	// user-created agent running the generic prompt path.
	Builtin string `json:"builtin,omitempty"`
	Source  string `json:"source"`

	// Inputs is the validated interview result — the same keys agentschema
	// declares, so a run can be replayed or explained after the fact.
	Inputs json.RawMessage `json:"inputs,omitempty"`

	Status string `json:"status"`
	// ExternalRunID names the specialist row doing the actual work — a
	// blog_runs, research_runs, pentest_runs, review_runs or task_runs id.
	// Kept as text because those tables are not otherwise related and a typed
	// foreign key would have to point at five of them.
	ExternalRunID *string `json:"external_run_id,omitempty"`
	// ExternalKind names which table ExternalRunID belongs to, so a reader does
	// not have to guess from the builtin.
	ExternalKind string  `json:"external_kind,omitempty"`
	ErrorMessage *string `json:"error_message,omitempty"`

	StartedAt   *time.Time `json:"started_at,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// IsTerminal reports whether the run has finished, either way.
func (r *AgentRun) IsTerminal() bool {
	return r != nil && (r.Status == AgentRunCompleted || r.Status == AgentRunFailed)
}

// CreateAgentRunRequest is the body of POST /api/v1/agent-runs.
//
// Inputs are untyped here on purpose: the schema that validates them belongs to
// the agent, and agentschema resolves it from the agent's builtin marker. A
// typed request per agent would put the fan-out back in the caller, which is
// the thing this endpoint exists to remove.
type CreateAgentRunRequest struct {
	AgentID uuid.UUID         `json:"agent_id"`
	TaskID  *uuid.UUID        `json:"task_id,omitempty"`
	Inputs  map[string]string `json:"inputs,omitempty"`
}

// AgentRunAccepted is the 202 body: enough to poll and to render a card.
type AgentRunAccepted struct {
	Run   *AgentRun `json:"run"`
	Agent string    `json:"agent"`
	Kind  string    `json:"kind"`
}

// AgentRunMissingInput is the 400 body when a required slot is empty.
//
// It is the same shape chat already renders as a clarifying question, so both
// surfaces can ask the user the same thing in their own idiom.
type AgentRunMissingInput struct {
	Missing  []string        `json:"missing"`
	Question string          `json:"question"`
	Options  []ClarifyOption `json:"options,omitempty"`
}
