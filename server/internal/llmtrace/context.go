// Package llmtrace traces every llm.Client call to a self-hosted Langfuse.
//
// It mirrors the python-sidecar's integration (python-sidecar/app/observability.py):
// tracing is on only when the Langfuse host and both API keys are configured,
// and when off every code path behaves exactly as before — Wrap returns the
// client unchanged and Shutdown is a no-op. Telemetry must never fail a
// request: spans are exported by a background batcher, and export errors are
// logged, not returned.
package llmtrace

import "context"

// TraceInfo labels every LLM call made under a ctx for Langfuse. The fields
// map onto the same conventions the python-sidecar established (see
// docs/langfuse.md): SessionID groups the calls of one run, AgentID feeds the
// dashboard's by-agent widget, TraceName names the engine that ran.
type TraceInfo struct {
	// TraceName is the engine label, e.g. "go-executor-run" or "go-blog-run",
	// following the sidecar's <engine>-<mode> pattern.
	TraceName string
	// SessionID is the execution/run/goal ID, so the calls of one run group
	// together in Langfuse's session view.
	SessionID string
	// AgentID becomes the Langfuse user, which the dashboard slices call
	// volume by. Empty means the caller has no agent (e.g. chat routing).
	AgentID string
	// OrgID tags the trace with the tenant. Empty when unknown.
	OrgID string
	// TaskID is the board task the run was launched from, when it has one.
	TaskID string
	// TaskRunID is the Task Manager run (task_runs.id) that started this
	// work, when it was started from one.
	TaskRunID string
	// ExecutionID is the agent execution (agent_executions.id) the calls
	// belong to, for executor runs.
	ExecutionID string
	// RunKind is the TraceName the run was started under. WithTrace sets it;
	// WithTraceName leaves it alone, so a nested engine (research inside a
	// blog run) still reports which run its calls belong to.
	RunKind string
}

type ctxKey struct{}

// WithTrace labels ctx so every LLM call under it is traced as info describes.
//
// The task link is inherited: a run started on behalf of a Task Manager run
// (WithTaskRun) keeps that task and task run unless info names its own.
func WithTrace(ctx context.Context, info TraceInfo) context.Context {
	if info.RunKind == "" {
		info.RunKind = info.TraceName
	}
	if outer, ok := FromContext(ctx); ok {
		if info.TaskID == "" {
			info.TaskID = outer.TaskID
		}
		if info.TaskRunID == "" {
			info.TaskRunID = outer.TaskRunID
		}
	}
	return context.WithValue(ctx, ctxKey{}, info)
}

// WithTaskRun records that the work under ctx was started by Task Manager run
// taskRunID on board task taskID. The run's own WithTrace, set later by the
// engine, inherits both.
func WithTaskRun(ctx context.Context, taskID, taskRunID string) context.Context {
	info, _ := FromContext(ctx)
	info.TaskID = taskID
	info.TaskRunID = taskRunID
	return context.WithValue(ctx, ctxKey{}, info)
}

// WithTraceName overrides only the engine label, keeping any session, agent
// and org already on the ctx. Research runs inside the blog pipeline use this
// so their calls stay grouped under the blog run's session while still being
// distinguishable as research.
func WithTraceName(ctx context.Context, name string) context.Context {
	info, _ := FromContext(ctx)
	info.TraceName = name
	return context.WithValue(ctx, ctxKey{}, info)
}

// FromContext returns the TraceInfo on ctx, if any.
func FromContext(ctx context.Context) (TraceInfo, bool) {
	info, ok := ctx.Value(ctxKey{}).(TraceInfo)
	return info, ok
}
