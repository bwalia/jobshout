// Package chatstream carries a per-request event emitter on the context so a
// synchronous execution path (chat -> router -> execution -> executor) can
// stream safe, high-level progress to an SSE handler without any layer changing
// its function signatures. The emitter is optional: when absent, Emit is a
// no-op and behaviour is exactly as before.
//
// Events are SAFE by construction — high-level states and tool activity
// (name/outcome/duration), never model reasoning or tool inputs/outputs.
package chatstream

import "context"

// Event types.
const (
	EventStatus = "status" // {state: planning|agent_selected|executing|completed|failed, ...}
	EventTool   = "tool"   // {name, state: start|end, duration_ms?, ok?}
	EventError  = "error"  // {message}
)

// Event is one progress update in a streamed turn.
type Event struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data,omitempty"`
}

// Emitter receives events. Implementations must be safe to call from the
// executing goroutine and must not block (SSE handlers use a buffered,
// drop-on-full channel).
type Emitter func(Event)

type emitterKey struct{}

// WithEmitter returns a context that carries em for downstream Emit calls.
func WithEmitter(ctx context.Context, em Emitter) context.Context {
	if em == nil {
		return ctx
	}
	return context.WithValue(ctx, emitterKey{}, em)
}

// Emit delivers ev to the context's emitter, or does nothing if none is set.
func Emit(ctx context.Context, ev Event) {
	if em, ok := ctx.Value(emitterKey{}).(Emitter); ok && em != nil {
		em(ev)
	}
}

// Status is a convenience for a status event with a state and optional fields.
func Status(ctx context.Context, state string, extra map[string]any) {
	data := map[string]any{"state": state}
	for k, v := range extra {
		data[k] = v
	}
	Emit(ctx, Event{Type: EventStatus, Data: data})
}
