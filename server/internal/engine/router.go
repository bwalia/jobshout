// Package engine provides a unified execution router that dispatches agent
// tasks to the appropriate runtime: Go native (ReAct), LangChain, or LangGraph.
package engine

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
)

// Runner is the interface that all execution engines must implement.
// The existing executor.Executor already satisfies this interface via its Run method.
type Runner interface {
	Run(ctx context.Context, execID uuid.UUID, agent *model.Agent, taskPrompt string, agentTools []string) executor.Result
}

// ToolGate reports whether a tool needs a human decision before an agent may run
// it. Satisfied by the approval service (and executor.ApprovalGate).
type ToolGate interface {
	RequiresApproval(agentID uuid.UUID, toolName string) bool
}

// Router selects the correct Runner based on engine type.
type Router struct {
	goNative  Runner
	langchain Runner
	langgraph Runner
	gate      ToolGate
	logger    *zap.Logger
}

// WithApprovalGate makes the Python-sidecar engines (LangChain, LangGraph)
// refuse to run an agent that has any approval-gated tool. Only the go_native
// executor can pause a run for a human decision and resume it; the sidecar
// would run the agent's tools with no gate at all. Refusing — rather than
// silently switching the run to go_native — keeps the engine an explicit
// choice: the error says to use go_native. Returns the receiver.
func (r *Router) WithApprovalGate(g ToolGate) *Router {
	r.gate = g
	return r
}

// NewRouter creates an engine Router.
// langchain and langgraph runners may be nil if the Python sidecar is not configured;
// in that case, requests for those engines return an error result.
func NewRouter(goNative Runner, langchain Runner, langgraph Runner, logger *zap.Logger) *Router {
	return &Router{
		goNative:  goNative,
		langchain: langchain,
		langgraph: langgraph,
		logger:    logger,
	}
}

// For returns the Runner for the given engine type.
func (r *Router) For(engineType string) Runner {
	switch engineType {
	case model.EngineLangChain:
		if r.langchain != nil {
			return r.guard(engineType, r.langchain)
		}
		return &unavailableRunner{engine: engineType}
	case model.EngineLangGraph:
		if r.langgraph != nil {
			return r.guard(engineType, r.langgraph)
		}
		return &unavailableRunner{engine: engineType}
	default:
		return r.goNative
	}
}

// ResolveEngine determines the effective engine type for a given agent + optional overrides.
func ResolveEngine(agent *model.Agent, requestOverride *string, stepOverride string) string {
	if requestOverride != nil && *requestOverride != "" {
		return *requestOverride
	}
	if stepOverride != "" {
		return stepOverride
	}
	if agent.EngineType != "" {
		return agent.EngineType
	}
	return model.EngineGoNative
}

// unavailableRunner returns an error for engines that are not configured.
type unavailableRunner struct {
	engine string
}

func (u *unavailableRunner) Run(_ context.Context, _ uuid.UUID, _ *model.Agent, _ string, _ []string) executor.Result {
	return executor.Result{
		Err: fmt.Errorf("engine %q is not available: Python sidecar not configured", u.engine),
	}
}

// guard wraps a sidecar runner so gated tools are refused there (see
// WithApprovalGate). Without a gate the runner is returned unchanged.
func (r *Router) guard(engineType string, inner Runner) Runner {
	if r.gate == nil {
		return inner
	}
	return &gatedSidecarRunner{engine: engineType, inner: inner, gate: r.gate}
}

// gatedSidecarRunner refuses to start a sidecar run whose agent may use a tool
// that requires human approval, since the sidecar cannot pause for one.
type gatedSidecarRunner struct {
	engine string
	inner  Runner
	gate   ToolGate
}

func (g *gatedSidecarRunner) Run(ctx context.Context, execID uuid.UUID, agent *model.Agent, taskPrompt string, agentTools []string) executor.Result {
	for _, name := range agentTools {
		if g.gate.RequiresApproval(agent.ID, name) {
			return executor.Result{Err: fmt.Errorf(
				"engine %q cannot run this agent: tool %q requires human approval, which only the %q engine supports; run the agent on %q",
				g.engine, name, model.EngineGoNative, model.EngineGoNative)}
		}
	}
	return g.inner.Run(ctx, execID, agent, taskPrompt, agentTools)
}
