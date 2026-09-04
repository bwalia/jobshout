package chatagent

import "github.com/jobshout/server/internal/model"

// EventType is an SSE event name for a streaming chat turn.
const (
	EventToken        = "token"
	EventToolCall     = "tool_call"
	EventToolResult   = "tool_result"
	EventConfirmation = "confirmation"
	EventClarify      = "clarify"
	EventDone         = "done"
	EventError        = "error"
	// EventModel names the model serving this turn. Emitted before the first
	// call and again if the client falls back to another model mid-turn.
	EventModel = "model"
)

// Event is one step of a streaming chat turn.
type Event struct {
	Type         string                 `json:"type"`
	Token        string                 `json:"token,omitempty"`
	Tool         string                 `json:"tool,omitempty"`
	Label        string                 `json:"label,omitempty"`
	Args         map[string]any         `json:"args,omitempty"`
	Status       string                 `json:"status,omitempty"`
	DurationMs   int64                  `json:"duration_ms,omitempty"`
	Entity       *model.EntityRef       `json:"entity,omitempty"`
	Confirmation *model.ConfirmRequest  `json:"confirmation,omitempty"`
	Clarify      *model.ClarifyRequest  `json:"clarify,omitempty"`
	Response     *model.ChatResponse    `json:"response,omitempty"`
	Error        string                 `json:"error,omitempty"`
	Model        string                 `json:"model,omitempty"`
	Provider     string                 `json:"provider,omitempty"`
}

// StreamFunc receives events as a turn progresses. Nil is fine.
type StreamFunc func(Event)

func emit(fn StreamFunc, ev Event) {
	if fn != nil {
		fn(ev)
	}
}
