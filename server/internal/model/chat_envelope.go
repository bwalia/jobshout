package model

import "time"

// ChatResponse is the structured envelope every chat surface renders.
// Message is human prose: no UUIDs, HTTP verbs, URL paths, or Go errors.
type ChatResponse struct {
	Message      string          `json:"message"`
	Actions      []ActionRecord  `json:"actions"`
	Entities     []EntityRef     `json:"entities"`
	Confirmation *ConfirmRequest `json:"confirmation,omitempty"`
	Clarify      *ClarifyRequest `json:"clarify,omitempty"`
	Usage        *UsageInfo      `json:"usage,omitempty"`
}

// ActionRecord is one tool invocation that actually ran (or was denied /
// held for confirmation). If the agent's reply claims something was done,
// an ActionRecord with Status "ok" must exist.
type ActionRecord struct {
	Tool       string         `json:"tool"`
	Args       map[string]any `json:"args"`
	Status     string         `json:"status"` // ok | failed | denied | pending_confirmation
	ResultRef  *EntityRef     `json:"result_ref,omitempty"`
	Error      string         `json:"error,omitempty"`
	DurationMs int64          `json:"duration_ms"`
}

const (
	ActionOK                  = "ok"
	ActionFailed              = "failed"
	ActionDenied              = "denied"
	ActionPendingConfirmation = "pending_confirmation"
)

// EntityRef is a named, linkable thing the UI can render as a card.
// Label is what the user reads; ID and Href are for navigation.
// URL, when set (typically kind=image), is a fetchable picture the chat
// bubble can render inline — a stored /api/v1/images/file/… path or a data URL.
type EntityRef struct {
	Kind  string `json:"kind"`
	ID    string `json:"id"`
	Label string `json:"label"`
	Href  string `json:"href,omitempty"`
	URL   string `json:"url,omitempty"`
}

// Entity kinds used by platform tools. Unknown kinds fall back to a plain link.
const (
	EntityAgent       = "agent"
	EntityExecution   = "execution"
	EntityTask        = "task"
	EntityProject     = "project"
	EntityWorkflow    = "workflow"
	EntityWorkflowRun = "workflow_run"
	EntityGoal        = "goal"
	EntityArticle     = "article_run"
	EntityPentest     = "pentest_run"
	EntityImage       = "image"
	EntitySprint      = "sprint"
	EntitySkill       = "skill"
	EntityPlugin      = "plugin"
	EntitySchedule    = "schedule"
)

// ConfirmRequest is a destructive action waiting for an explicit Approve.
type ConfirmRequest struct {
	Token     string `json:"token"`
	Tool      string `json:"tool"`
	Summary   string `json:"summary"`
	Effect    string `json:"effect"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

// ClarifyRequest asks the user to fill a missing slot.
type ClarifyRequest struct {
	Question string          `json:"question"`
	Slot     string          `json:"slot,omitempty"`
	Options  []ClarifyOption `json:"options,omitempty"`
}

// ClarifyOption is a quick-reply chip.
type ClarifyOption struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// UsageInfo is optional token/cost telemetry for a turn.
type UsageInfo struct {
	Model        string  `json:"model,omitempty"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	LatencyMs    int64   `json:"latency_ms"`
	CostUSD      float64 `json:"cost_usd,omitempty"`
}

// SessionEntity is a pronoun-resolution referent stored on the session.
type SessionEntity struct {
	ID    string    `json:"id"`
	Label string    `json:"label"`
	Kind  string    `json:"kind"`
	Href  string    `json:"href,omitempty"`
	At    time.Time `json:"at"`
}

// PendingAction is a tool call waiting for a missing argument.
type PendingAction struct {
	Tool     string         `json:"tool"`
	Args     map[string]any `json:"args"`
	Missing  []string       `json:"missing"`
	AskedAt  time.Time      `json:"asked_at"`
	Question string         `json:"question,omitempty"`
}

// PendingConfirmation is a destructive tool waiting for an Approve click.
type PendingConfirmation struct {
	Token     string         `json:"token"`
	Tool      string         `json:"tool"`
	Args      map[string]any `json:"args"`
	Summary   string         `json:"summary"`
	Effect    string         `json:"effect"`
	ExpiresAt time.Time      `json:"expires_at"`
}

// Chat session metadata keys. All chat continuity state lives here so a
// restarted process can reconstruct a turn from Postgres alone.
const (
	ChatMetaSummary       = "summary"
	ChatMetaEntities      = "entities"
	ChatMetaPending       = "pending_action"
	ChatMetaConfirmation  = "pending_confirmation"
	ChatMetaLastKind      = "last_entity_kind"
	ChatMetaTurnStartedAt = "turn_started_at"
)

// ChatTurnResult is what ChatService returns after a send: persisted
// messages plus the structured envelope the UI renders.
type ChatTurnResult struct {
	UserMessage  *ChatMessage `json:"user_message"`
	AgentMessage *ChatMessage `json:"agent_message"`
	Response     ChatResponse `json:"response"`
}
