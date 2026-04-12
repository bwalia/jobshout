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

// Router selects the correct Runner based on engine type.
type Router struct {
	goNative  Runner
	langchain Runner
	langgraph Runner
	logger    *zap.Logger
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
			return r.langchain
		}
		return &unavailableRunner{engine: engineType}
	case model.EngineLangGraph:
		if r.langgraph != nil {
			return r.langgraph
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
