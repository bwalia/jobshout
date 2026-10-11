package model

import (
	"context"
	"strings"
)

// RunContext is the caller-supplied role context of a run: who the agent is
// acting for and as, so one agent can serve several roles (e.g. OpsAPI's
// "seller_agent" vs "manager"). It is stored on the run (task_runs.run_context),
// echoed in run webhooks, and rendered into the run prompt's Inputs block.
// Fields are opaque references owned by the caller; JobShout never resolves
// them.
type RunContext struct {
	// Role the agent acts in for this run, e.g. "seller_agent", "manager".
	Role string `json:"role,omitempty" validate:"omitempty,max=64"`
	// UserRef identifies the caller's end user (an id in the caller's system).
	UserRef string `json:"user_ref,omitempty" validate:"omitempty,max=200"`
	// WorkspaceRef identifies the caller's workspace/tenant/property.
	WorkspaceRef string `json:"workspace_ref,omitempty" validate:"omitempty,max=200"`
}

// Normalize trims the fields and returns nil when all are empty.
func (c *RunContext) Normalize() *RunContext {
	if c == nil {
		return nil
	}
	out := RunContext{
		Role:         strings.TrimSpace(c.Role),
		UserRef:      strings.TrimSpace(c.UserRef),
		WorkspaceRef: strings.TrimSpace(c.WorkspaceRef),
	}
	if out == (RunContext{}) {
		return nil
	}
	return &out
}

// MergeInto adds the context to a run's inputs under the keys "role",
// "user_ref" and "workspace_ref" (the context wins over same-named inputs), so
// the run prompt's "## Inputs" block tells the agent its role. inputs may be nil.
func (c *RunContext) MergeInto(inputs map[string]any) map[string]any {
	if c == nil {
		return inputs
	}
	if inputs == nil {
		inputs = map[string]any{}
	}
	if c.Role != "" {
		inputs["role"] = c.Role
	}
	if c.UserRef != "" {
		inputs["user_ref"] = c.UserRef
	}
	if c.WorkspaceRef != "" {
		inputs["workspace_ref"] = c.WorkspaceRef
	}
	return inputs
}

type runContextKey struct{}

// WithRunContext carries a run context to the run's creation (the task-run
// repository stores it on the new run).
func WithRunContext(ctx context.Context, c *RunContext) context.Context {
	if c == nil {
		return ctx
	}
	return context.WithValue(ctx, runContextKey{}, c)
}

// RunContextFrom returns the run context set by WithRunContext, or nil.
func RunContextFrom(ctx context.Context) *RunContext {
	c, _ := ctx.Value(runContextKey{}).(*RunContext)
	return c
}
