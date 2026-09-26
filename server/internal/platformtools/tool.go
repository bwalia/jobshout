package platformtools

import (
	"context"
	"encoding/json"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/tools"
)

// PlatformTool extends tools.Tool with the metadata the chat guard chain needs.
// Agent-facing tools keep implementing tools.Tool only; this interface is
// additive and lives in a separate registry.
type PlatformTool interface {
	tools.Tool
	Schema() map[string]any
	Domain() string
	Permission() string
	Destructive() bool
	ReadOnly() bool
	// Run is the typed execute path the chat loop uses. Execute (from
	// tools.Tool) is implemented as a JSON wrapper around Run so a
	// PlatformTool can still sit in a tools.Registry if needed.
	Run(ctx context.Context, input map[string]any) (*Result, error)
}

// Result is what a platform tool returns to the chat loop. Data is serialised
// into an untrusted delimiter block for the model. Entity/Entities populate
// the response envelope. Missing triggers slot-filling rather than prose.
type Result struct {
	Data     any
	Entity   *model.EntityRef
	Entities []model.EntityRef
	Missing  []string
	Options  []model.ClarifyOption
	Question string
	// Effect is a concrete description of a destructive action, used when
	// the guard holds the call for confirmation.
	Effect string
}

// Registry holds platform tools keyed by name.
type Registry struct {
	tools map[string]PlatformTool
}

func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]PlatformTool)}
}

func (r *Registry) Register(t PlatformTool) {
	if _, exists := r.tools[t.Name()]; exists {
		panic("platformtools: duplicate tool name: " + t.Name())
	}
	r.tools[t.Name()] = t
}

func (r *Registry) Get(name string) (PlatformTool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

func (r *Registry) All() []PlatformTool {
	out := make([]PlatformTool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

func (r *Registry) Names() []string {
	out := make([]string, 0, len(r.tools))
	for name := range r.tools {
		out = append(out, name)
	}
	return out
}

// FilterByPermissions returns tools the caller is allowed to see. Empty
// permission on a tool means any authenticated user. If allowed is nil,
// every tool is returned (used in tests without an RBAC service).
func (r *Registry) FilterByPermissions(allowed map[string]bool) []PlatformTool {
	out := make([]PlatformTool, 0, len(r.tools))
	for _, t := range r.tools {
		if t.Permission() == "" {
			out = append(out, t)
			continue
		}
		if allowed == nil || allowed[t.Permission()] {
			out = append(out, t)
		}
	}
	return out
}

func marshalJSON(v any) string {
	if v == nil {
		return "{}"
	}
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"failed to serialise tool result"}`
	}
	return string(b)
}
