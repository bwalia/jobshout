package tools

import (
	"context"

	"github.com/google/uuid"
)

type ctxKey int

const (
	orgCtxKey ctxKey = iota
	agentCtxKey
)

// WithOrg returns a context carrying the organization id. Org-scoped tools (e.g.
// the integration tools) read it to resolve the calling tenant's own configured
// credentials at execution time, so one registered tool instance serves every
// organization. The executor injects this before invoking any tool.
func WithOrg(ctx context.Context, orgID uuid.UUID) context.Context {
	return context.WithValue(ctx, orgCtxKey, orgID)
}

// OrgFromContext returns the organization id previously stored with WithOrg.
func OrgFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(orgCtxKey).(uuid.UUID)
	return v, ok
}

// WithAgent returns a context carrying the executing agent's id. Agent-scoped
// tools (e.g. knowledge_search) read it to resolve the agent's own knowledge
// base at execution time, so one registered tool instance serves every agent.
// The executor injects this before invoking any tool, alongside WithOrg.
func WithAgent(ctx context.Context, agentID uuid.UUID) context.Context {
	return context.WithValue(ctx, agentCtxKey, agentID)
}

// AgentFromContext returns the agent id previously stored with WithAgent.
func AgentFromContext(ctx context.Context) (uuid.UUID, bool) {
	v, ok := ctx.Value(agentCtxKey).(uuid.UUID)
	return v, ok
}
