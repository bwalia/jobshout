// Package tools provides the Tool interface and a Registry that agents use to
// discover and invoke capabilities during the ReAct execution loop.
package tools

import "context"

// Tool represents a capability an agent can invoke during task execution.
type Tool interface {
	// Name is the unique identifier used by the LLM when selecting this tool.
	Name() string
	// Description explains what the tool does and when to use it. This text is
	// included verbatim in the agent system prompt.
	Description() string
	// Execute runs the tool with the provided input map and returns a string
	// result that is fed back into the LLM context.
	Execute(ctx context.Context, input map[string]any) (string, error)
}

// Registry holds all available tools and allows lookup by name.
type Registry struct {
	tools map[string]Tool
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]Tool)}
}

// Register adds a tool to the registry. Panics on duplicate names to catch
// configuration errors at startup.
func (r *Registry) Register(t Tool) {
	if _, exists := r.tools[t.Name()]; exists {
		panic("tools: duplicate tool name registered: " + t.Name())
	}
	r.tools[t.Name()] = t
}

// Get returns the tool with the given name, or (nil, false) if not found.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// Subset returns a new Registry containing only tools whose names appear in
// the allowList slice. Tools not in the allowList are silently excluded.
func (r *Registry) Subset(allowList []string) *Registry {
	sub := NewRegistry()
	for _, name := range allowList {
		if t, ok := r.tools[name]; ok {
			sub.tools[name] = t
		}
	}
	return sub
}

// All returns all registered tools in unspecified order.
func (r *Registry) All() []Tool {
	out := make([]Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}
