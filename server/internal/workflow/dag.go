// Package workflow provides the DAG-based multi-agent workflow execution engine.
// Steps without dependencies are executed concurrently; dependent steps wait
// for all their prerequisites before starting.
package workflow

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
)

// StepResult holds the output (or error) of a single workflow step.
type StepResult struct {
	StepName string
	Output   string
	Err      error
}

// AgentResolver fetches an Agent record by ID — supplied by the caller to
// avoid a hard dependency on the repository layer from within this package.
type AgentResolver func(ctx context.Context, agentID uuid.UUID) (*model.Agent, error)

// ToolPermissionResolver returns the list of tool names the agent may use.
type ToolPermissionResolver func(ctx context.Context, agentID uuid.UUID) ([]string, error)

// ExecutionPersister persists the start and finish of each agent execution.
// Returning an error from either method is non-fatal — the DAG continues.
type ExecutionPersister interface {
	RecordStarted(ctx context.Context, execID uuid.UUID, agentID uuid.UUID, orgID uuid.UUID, runID uuid.UUID, stepID uuid.UUID, prompt string) error
	RecordCompleted(ctx context.Context, execID uuid.UUID, result executor.Result) error
}

// Engine executes a workflow definition against a concrete Executor.
type Engine struct {
	exec                  *executor.Executor
	resolveAgent          AgentResolver
	resolveToolPermissions ToolPermissionResolver
	persister             ExecutionPersister
	logger                *zap.Logger
}

// NewEngine creates a DAG Engine.
func NewEngine(
	exec *executor.Executor,
	resolveAgent AgentResolver,
	resolveToolPermissions ToolPermissionResolver,
	persister ExecutionPersister,
	logger *zap.Logger,
) *Engine {
	return &Engine{
		exec:                  exec,
		resolveAgent:          resolveAgent,
		resolveToolPermissions: resolveToolPermissions,
		persister:             persister,
		logger:                logger,
	}
}

// Execute runs the workflow steps respecting their dependency graph.
// It returns a map of step name → output text, and any fatal error.
func (e *Engine) Execute(
	ctx context.Context,
	wf *model.Workflow,
	run *model.WorkflowRun,
	globalInput map[string]any,
) (map[string]string, error) {
	if len(wf.Steps) == 0 {
		return map[string]string{}, nil
	}

	// Build adjacency data: stepName → step, and dep counts.
	byName := make(map[string]*model.WorkflowStep, len(wf.Steps))
	for i := range wf.Steps {
		byName[wf.Steps[i].Name] = &wf.Steps[i]
	}

	// pending tracks how many unsatisfied dependencies each step has.
	pending := make(map[string]int, len(wf.Steps))
	// dependents maps step name → names of steps that depend on it.
	dependents := make(map[string][]string)

	for _, step := range wf.Steps {
		pending[step.Name] = len(step.DependsOn)
		for _, dep := range step.DependsOn {
			dependents[dep] = append(dependents[dep], step.Name)
		}
	}

	// Shared outputs map, protected by a mutex since goroutines write concurrently.
	var mu sync.Mutex
	outputs := make(map[string]string)

	// ready is a channel of step names that are cleared to execute.
	ready := make(chan string, len(wf.Steps))

	// Seed the channel with steps that have no dependencies.
	for _, step := range wf.Steps {
		if len(step.DependsOn) == 0 {
			ready <- step.Name
		}
	}

	var wg sync.WaitGroup
	errs := make(chan error, len(wf.Steps))

	// Track how many steps are in flight or not yet started.
	remaining := len(wf.Steps)

	for remaining > 0 {
		select {
		case <-ctx.Done():
			return outputs, ctx.Err()

		case stepName := <-ready:
			remaining--
			step := byName[stepName]

			wg.Add(1)
			go func(s *model.WorkflowStep) {
				defer wg.Done()

				mu.Lock()
				currentOutputs := copyMap(outputs)
				mu.Unlock()

				prompt := renderTemplate(s.InputTemplate, globalInput, currentOutputs)

				e.logger.Info("starting workflow step",
					zap.String("workflow", wf.Name),
					zap.String("step", s.Name),
					zap.String("agent_id", s.AgentID.String()),
				)

				agent, err := e.resolveAgent(ctx, s.AgentID)
				if err != nil {
					errs <- fmt.Errorf("step %q: resolve agent: %w", s.Name, err)
					// Still need to unblock dependents to avoid a deadlock;
					// they will receive an empty output.
					e.unblockDependents(s.Name, dependents, pending, ready, &mu)
					return
				}

				agentTools, err := e.resolveToolPermissions(ctx, s.AgentID)
				if err != nil {
					agentTools = []string{} // safe fallback — no tools
				}

				execID := uuid.New()
				_ = e.persister.RecordStarted(ctx, execID, agent.ID, run.OrgID, run.ID, s.ID, prompt)

				res := e.exec.Run(ctx, execID, agent, prompt, agentTools)
				_ = e.persister.RecordCompleted(ctx, execID, res)

				if res.Err != nil {
					errs <- fmt.Errorf("step %q: agent execution failed: %w", s.Name, res.Err)
					e.unblockDependents(s.Name, dependents, pending, ready, &mu)
					return
				}

				e.logger.Info("workflow step completed",
					zap.String("step", s.Name),
					zap.Int("iterations", res.Iterations),
				)

				mu.Lock()
				outputs[s.Name] = res.FinalAnswer
				mu.Unlock()

				e.unblockDependents(s.Name, dependents, pending, ready, &mu)
			}(step)
		}
	}

	wg.Wait()
	close(errs)

	// Collect all errors.
	var allErrs []string
	for err := range errs {
		allErrs = append(allErrs, err.Error())
	}
	if len(allErrs) > 0 {
		return outputs, fmt.Errorf("workflow %q encountered errors: %s", wf.Name, strings.Join(allErrs, "; "))
	}

	return outputs, nil
}

// unblockDependents decrements the pending count for each step that depends on
// completedStep, and enqueues any that become ready (pending == 0).
func (e *Engine) unblockDependents(
	completedStep string,
	dependents map[string][]string,
	pending map[string]int,
	ready chan<- string,
	mu *sync.Mutex,
) {
	mu.Lock()
	defer mu.Unlock()

	for _, dep := range dependents[completedStep] {
		pending[dep]--
		if pending[dep] == 0 {
			ready <- dep
		}
	}
}

// renderTemplate performs simple substitution of {{.Input.<key>}} and
// {{.Outputs.<step_name>}} placeholders in the step input template.
func renderTemplate(tmpl string, input map[string]any, outputs map[string]string) string {
	result := tmpl

	for k, v := range input {
		result = strings.ReplaceAll(result, fmt.Sprintf("{{.Input.%s}}", k), fmt.Sprintf("%v", v))
	}
	for stepName, out := range outputs {
		result = strings.ReplaceAll(result, fmt.Sprintf("{{.Outputs.%s}}", stepName), out)
	}
	return result
}

func copyMap(m map[string]string) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
