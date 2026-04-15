package executor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
)

// MaxGoalIterations caps how many plan+act cycles a goal can run.
const MaxGoalIterations = 5

// MemoryStore is the subset of repository.MemoryRepository needed by the executor.
// Defined here to avoid an import cycle (repository → executor → repository).
type MemoryStore interface {
	AppendLongTerm(ctx context.Context, mem *model.AgentMemoryLongTerm) error
	SearchLongTerm(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]model.AgentMemoryLongTerm, error)
}

// GoalStore is the subset of repository.GoalRepository needed by the executor.
type GoalStore interface {
	GetByID(ctx context.Context, id uuid.UUID) (*model.AgentGoal, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdatePlan(ctx context.Context, id uuid.UUID, plan []model.PlanStep) error
	MarkStarted(ctx context.Context, id uuid.UUID) error
	MarkCompleted(ctx context.Context, id uuid.UUID, reflection string) error
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error
	IncrementIteration(ctx context.Context, id uuid.UUID) error
}

// GoalResult holds the outcome of an autonomous goal execution.
type GoalResult struct {
	GoalID      uuid.UUID
	Plan        []model.PlanStep
	Observations []string
	Reflection  string
	Iterations  int
	Err         error
}

// AutonomousExecutor wraps the base ReAct Executor to provide a
// Goal → Plan → Act → Observe → Reflect loop.
type AutonomousExecutor struct {
	base     *Executor
	llm      *llm.Router
	memory   MemoryStore
	goalRepo GoalStore
	logger   *zap.Logger
}

// NewAutonomousExecutor creates an AutonomousExecutor that delegates Act-phase
// execution to the existing ReAct executor.
func NewAutonomousExecutor(
	base *Executor,
	llmRouter *llm.Router,
	memoryRepo MemoryStore,
	goalRepo GoalStore,
	logger *zap.Logger,
) *AutonomousExecutor {
	return &AutonomousExecutor{
		base:     base,
		llm:      llmRouter,
		memory:   memoryRepo,
		goalRepo: goalRepo,
		logger:   logger,
	}
}

// RunGoal executes the full autonomous loop for a goal.
func (a *AutonomousExecutor) RunGoal(
	ctx context.Context,
	goalID uuid.UUID,
	agent *model.Agent,
	agentTools []string,
) GoalResult {
	log := a.logger.With(
		zap.String("goal_id", goalID.String()),
		zap.String("agent_id", agent.ID.String()),
	)

	goal, err := a.goalRepo.GetByID(ctx, goalID)
	if err != nil {
		return GoalResult{GoalID: goalID, Err: fmt.Errorf("autonomous: load goal: %w", err)}
	}

	// ── Phase 1: Plan ────────────────────────────────────────────────────────
	log.Info("phase: planning")
	if err := a.goalRepo.UpdateStatus(ctx, goalID, model.GoalStatusPlanning); err != nil {
		log.Warn("failed to update goal status to planning", zap.Error(err))
	}

	// Recall relevant long-term memories.
	var memories []string
	if mems, err := a.memory.SearchLongTerm(ctx, agent.ID, goal.GoalText, 5); err == nil {
		for _, m := range mems {
			memories = append(memories, m.Summary)
		}
	}

	plan, err := a.generatePlan(ctx, agent, goal.GoalText, memories)
	if err != nil {
		errMsg := fmt.Sprintf("planning failed: %v", err)
		_ = a.goalRepo.MarkFailed(ctx, goalID, errMsg)
		return GoalResult{GoalID: goalID, Err: err}
	}

	if err := a.goalRepo.UpdatePlan(ctx, goalID, plan); err != nil {
		log.Warn("failed to persist plan", zap.Error(err))
	}

	// ── Phase 2: Act + Observe (per step) ────────────────────────────────────
	log.Info("phase: executing", zap.Int("steps", len(plan)))
	if err := a.goalRepo.MarkStarted(ctx, goalID); err != nil {
		log.Warn("failed to mark goal started", zap.Error(err))
	}

	var observations []string
	for i, step := range plan {
		if ctx.Err() != nil {
			_ = a.goalRepo.MarkFailed(ctx, goalID, "context cancelled")
			return GoalResult{GoalID: goalID, Plan: plan, Observations: observations, Err: ctx.Err()}
		}

		log.Info("executing step", zap.Int("step", i), zap.String("description", step.Description))
		_ = a.goalRepo.IncrementIteration(ctx, goalID)

		// Build a focused prompt for this step, including prior observations.
		stepPrompt := a.buildStepPrompt(goal.GoalText, step, observations)

		// Delegate to the base ReAct executor.
		execID := uuid.New()
		result := a.base.Run(ctx, execID, agent, stepPrompt, agentTools)

		observation := result.FinalAnswer
		if result.Err != nil {
			observation = fmt.Sprintf("Error: %v", result.Err)
			log.Warn("step execution failed", zap.Int("step", i), zap.Error(result.Err))
		}

		observations = append(observations, observation)
		plan[i].Completed = true
		plan[i].Output = truncate(observation, 2000)

		// Persist progress after each step.
		_ = a.goalRepo.UpdatePlan(ctx, goalID, plan)

		// Store observation as long-term memory.
		_ = a.memory.AppendLongTerm(ctx, &model.AgentMemoryLongTerm{
			AgentID: agent.ID,
			OrgID:   agent.OrgID,
			Content: observation,
			Summary: fmt.Sprintf("Step %d of goal %q: %s", i+1, truncate(goal.GoalText, 100), step.Description),
		})
	}

	// ── Phase 3: Reflect ─────────────────────────────────────────────────────
	log.Info("phase: reflecting")
	if err := a.goalRepo.UpdateStatus(ctx, goalID, model.GoalStatusReflecting); err != nil {
		log.Warn("failed to update goal status to reflecting", zap.Error(err))
	}

	reflection, err := a.generateReflection(ctx, agent, goal.GoalText, observations)
	if err != nil {
		log.Warn("reflection generation failed, using fallback", zap.Error(err))
		reflection = "Reflection could not be generated. Steps were executed successfully."
	}

	if err := a.goalRepo.MarkCompleted(ctx, goalID, reflection); err != nil {
		log.Error("failed to mark goal completed", zap.Error(err))
	}

	log.Info("goal completed", zap.Int("steps", len(plan)))
	return GoalResult{
		GoalID:       goalID,
		Plan:         plan,
		Observations: observations,
		Reflection:   reflection,
		Iterations:   len(plan),
	}
}

// generatePlan calls the LLM to decompose a goal into concrete steps.
func (a *AutonomousExecutor) generatePlan(
	ctx context.Context,
	agent *model.Agent,
	goalText string,
	memories []string,
) ([]model.PlanStep, error) {
	client, err := a.resolveClient(agent)
	if err != nil {
		return nil, err
	}

	prompt := llm.BuildPlanningPrompt(agent.Name, agent.Role, goalText, memories)

	modelName := ""
	if agent.ModelName != nil {
		modelName = *agent.ModelName
	}

	resp, err := client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       modelName,
		MaxTokens:   2048,
		Temperature: 0.3,
	})
	if err != nil {
		return nil, fmt.Errorf("autonomous: plan generation: %w", err)
	}

	// Parse JSON response.
	type planResponse struct {
		Steps []model.PlanStep `json:"steps"`
	}

	content := extractJSON(resp.Content)
	var parsed planResponse
	if err := json.Unmarshal([]byte(content), &parsed); err != nil {
		// Fallback: single-step plan with the entire goal.
		a.logger.Warn("failed to parse plan JSON, using single-step fallback",
			zap.String("raw", resp.Content), zap.Error(err))
		return []model.PlanStep{
			{Index: 0, Description: goalText},
		}, nil
	}

	if len(parsed.Steps) == 0 {
		return []model.PlanStep{
			{Index: 0, Description: goalText},
		}, nil
	}

	return parsed.Steps, nil
}

// generateReflection calls the LLM to produce a summary of the goal execution.
func (a *AutonomousExecutor) generateReflection(
	ctx context.Context,
	agent *model.Agent,
	goalText string,
	observations []string,
) (string, error) {
	client, err := a.resolveClient(agent)
	if err != nil {
		return "", err
	}

	prompt := llm.BuildReflectionPrompt(agent.Name, goalText, observations)

	modelName := ""
	if agent.ModelName != nil {
		modelName = *agent.ModelName
	}

	resp, err := client.Generate(ctx, llm.GenerateRequest{
		Messages: []llm.Message{
			{Role: llm.RoleUser, Content: prompt},
		},
		Model:       modelName,
		MaxTokens:   1024,
		Temperature: 0.4,
	})
	if err != nil {
		return "", fmt.Errorf("autonomous: reflection generation: %w", err)
	}

	return strings.TrimSpace(resp.Content), nil
}

// buildStepPrompt creates the user prompt for a single plan step, including context.
func (a *AutonomousExecutor) buildStepPrompt(goalText string, step model.PlanStep, priorObservations []string) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("You are working toward the goal: %q\n\n", goalText))
	sb.WriteString(fmt.Sprintf("Current step (%d): %s\n", step.Index+1, step.Description))

	if step.ToolHint != "" {
		sb.WriteString(fmt.Sprintf("Suggested tool: %s\n", step.ToolHint))
	}

	if len(priorObservations) > 0 {
		sb.WriteString("\nPrevious step results:\n")
		for i, obs := range priorObservations {
			sb.WriteString(fmt.Sprintf("  Step %d: %s\n", i+1, truncate(obs, 500)))
		}
	}

	sb.WriteString("\nExecute this step and provide the result.")
	return sb.String()
}

func (a *AutonomousExecutor) resolveClient(agent *model.Agent) (llm.Client, error) {
	providerName := ""
	if agent.ModelProvider != nil {
		providerName = *agent.ModelProvider
	}
	return a.llm.For(providerName)
}

// extractJSON finds and returns the first JSON object in a string.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	s = strings.TrimSpace(s)

	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end < start {
		return s
	}
	return s[start : end+1]
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
