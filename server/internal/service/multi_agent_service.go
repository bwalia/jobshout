package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// MultiAgentService orchestrates planner → executor → reviewer collaboration.
type MultiAgentService interface {
	RunJob(ctx context.Context, orgID uuid.UUID, req model.RunMultiAgentRequest) (*model.MultiAgentJob, error)
	GetJob(ctx context.Context, id uuid.UUID) (*model.MultiAgentJob, error)
	ListJobs(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.MultiAgentJob], error)
}

type multiAgentService struct {
	repo         repository.MultiAgentRepository
	agentRepo    repository.AgentRepository
	toolPermRepo repository.AgentToolRepository
	goalRepo     repository.GoalRepository
	autoExec     *executor.AutonomousExecutor
	logger       *zap.Logger
}

func NewMultiAgentService(
	repo repository.MultiAgentRepository,
	agentRepo repository.AgentRepository,
	toolPermRepo repository.AgentToolRepository,
	goalRepo repository.GoalRepository,
	autoExec *executor.AutonomousExecutor,
	logger *zap.Logger,
) MultiAgentService {
	return &multiAgentService{
		repo:         repo,
		agentRepo:    agentRepo,
		toolPermRepo: toolPermRepo,
		goalRepo:     goalRepo,
		autoExec:     autoExec,
		logger:       logger,
	}
}

func (s *multiAgentService) RunJob(ctx context.Context, orgID uuid.UUID, req model.RunMultiAgentRequest) (*model.MultiAgentJob, error) {
	maxReview := req.MaxReview
	if maxReview <= 0 {
		maxReview = 2
	}

	job := &model.MultiAgentJob{
		ID:         uuid.New(),
		OrgID:      orgID,
		TaskPrompt: req.TaskPrompt,
		PlannerID:  req.PlannerID,
		ExecutorID: req.ExecutorID,
		ReviewerID: req.ReviewerID,
		Status:     model.MultiAgentStatusPending,
		MaxReview:  maxReview,
	}

	if err := s.repo.Create(ctx, job); err != nil {
		return nil, fmt.Errorf("multi_agent_svc: create: %w", err)
	}

	// Run the collaboration loop asynchronously.
	go func() {
		bgCtx := context.Background()
		s.runCollaboration(bgCtx, job)
	}()

	return job, nil
}

func (s *multiAgentService) GetJob(ctx context.Context, id uuid.UUID) (*model.MultiAgentJob, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *multiAgentService) ListJobs(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.MultiAgentJob], error) {
	return s.repo.ListByOrg(ctx, orgID, params)
}

func (s *multiAgentService) runCollaboration(ctx context.Context, job *model.MultiAgentJob) {
	log := s.logger.With(zap.String("job_id", job.ID.String()))

	// Load all three agents.
	planner, err := s.agentRepo.FindByID(ctx, job.PlannerID)
	if err != nil {
		s.failJob(ctx, job.ID, log, "load planner agent: "+err.Error())
		return
	}
	execAgent, err := s.agentRepo.FindByID(ctx, job.ExecutorID)
	if err != nil {
		s.failJob(ctx, job.ID, log, "load executor agent: "+err.Error())
		return
	}
	reviewer, err := s.agentRepo.FindByID(ctx, job.ReviewerID)
	if err != nil {
		s.failJob(ctx, job.ID, log, "load reviewer agent: "+err.Error())
		return
	}

	// Load executor tools.
	execTools, _ := s.toolPermRepo.ListByAgent(ctx, job.ExecutorID)

	// ── Phase 1: Planner creates the plan ────────────────────────────────────
	log.Info("multi-agent: planning phase")
	_ = s.repo.UpdateStatus(ctx, job.ID, model.MultiAgentStatusPlanning)

	planGoal := &model.AgentGoal{
		ID:       uuid.New(),
		AgentID:  planner.ID,
		OrgID:    job.OrgID,
		GoalText: fmt.Sprintf("Create a detailed step-by-step execution plan for: %s", job.TaskPrompt),
		Plan:     []model.PlanStep{},
		Status:   model.GoalStatusPending,
		MaxIter:  executor.MaxGoalIterations,
	}

	if err := s.goalRepo.Create(ctx, planGoal); err != nil {
		s.failJob(ctx, job.ID, log, "create plan goal: "+err.Error())
		return
	}

	planResult := s.autoExec.RunGoal(ctx, planGoal.ID, planner, nil)
	if planResult.Err != nil {
		s.failJob(ctx, job.ID, log, "planner failed: "+planResult.Err.Error())
		return
	}

	planOutput := planResult.Reflection
	if planOutput == "" && len(planResult.Observations) > 0 {
		planOutput = planResult.Observations[len(planResult.Observations)-1]
	}
	_ = s.repo.UpdatePlanOutput(ctx, job.ID, planOutput)

	// ── Phase 2+3: Executor runs, reviewer validates (with retry) ────────────
	for iteration := 0; iteration < job.MaxReview; iteration++ {
		_ = s.repo.IncrementIteration(ctx, job.ID)

		// Execute
		log.Info("multi-agent: execution phase", zap.Int("iteration", iteration))
		_ = s.repo.UpdateStatus(ctx, job.ID, model.MultiAgentStatusExecuting)

		execGoal := &model.AgentGoal{
			ID:       uuid.New(),
			AgentID:  execAgent.ID,
			OrgID:    job.OrgID,
			GoalText: fmt.Sprintf("Execute the following plan for task %q:\n\n%s", job.TaskPrompt, planOutput),
			Plan:     []model.PlanStep{},
			Status:   model.GoalStatusPending,
			MaxIter:  executor.MaxGoalIterations,
		}

		if err := s.goalRepo.Create(ctx, execGoal); err != nil {
			s.failJob(ctx, job.ID, log, "create exec goal: "+err.Error())
			return
		}

		execResult := s.autoExec.RunGoal(ctx, execGoal.ID, execAgent, execTools)
		execOutput := execResult.Reflection
		if execOutput == "" && len(execResult.Observations) > 0 {
			execOutput = execResult.Observations[len(execResult.Observations)-1]
		}
		_ = s.repo.UpdateExecOutput(ctx, job.ID, execOutput)

		// Review
		log.Info("multi-agent: review phase", zap.Int("iteration", iteration))
		_ = s.repo.UpdateStatus(ctx, job.ID, model.MultiAgentStatusReviewing)

		reviewGoal := &model.AgentGoal{
			ID:      uuid.New(),
			AgentID: reviewer.ID,
			OrgID:   job.OrgID,
			GoalText: fmt.Sprintf(
				"Review the execution result for task %q.\n\nPlan:\n%s\n\nExecution result:\n%s\n\n"+
					"Respond with a JSON object: {\"approved\": true/false, \"feedback\": \"...\"}",
				job.TaskPrompt, planOutput, execOutput,
			),
			Plan:    []model.PlanStep{},
			Status:  model.GoalStatusPending,
			MaxIter: 3,
		}

		if err := s.goalRepo.Create(ctx, reviewGoal); err != nil {
			s.failJob(ctx, job.ID, log, "create review goal: "+err.Error())
			return
		}

		reviewResult := s.autoExec.RunGoal(ctx, reviewGoal.ID, reviewer, nil)
		reviewOutput := reviewResult.Reflection
		if reviewOutput == "" && len(reviewResult.Observations) > 0 {
			reviewOutput = reviewResult.Observations[len(reviewResult.Observations)-1]
		}

		// Try to parse approval from the review output.
		approved := true
		type reviewDecision struct {
			Approved bool   `json:"approved"`
			Feedback string `json:"feedback"`
		}
		var decision reviewDecision
		if err := json.Unmarshal([]byte(extractReviewJSON(reviewOutput)), &decision); err == nil {
			approved = decision.Approved
			if decision.Feedback != "" {
				reviewOutput = decision.Feedback
			}
		}

		_ = s.repo.UpdateReviewOutput(ctx, job.ID, reviewOutput, approved)

		if approved {
			log.Info("multi-agent: approved")
			_ = s.repo.MarkCompleted(ctx, job.ID)
			return
		}

		log.Info("multi-agent: rejected, will retry", zap.String("feedback", reviewOutput))
		// Feed the feedback back into the next execution iteration.
		planOutput = fmt.Sprintf("%s\n\nReviewer feedback (iteration %d): %s", planOutput, iteration+1, reviewOutput)
	}

	// Exhausted review cycles — mark completed anyway with the last result.
	_ = s.repo.MarkCompleted(ctx, job.ID)
	log.Info("multi-agent: completed after max review cycles")
}

func (s *multiAgentService) failJob(ctx context.Context, id uuid.UUID, log *zap.Logger, msg string) {
	log.Error("multi-agent job failed", zap.String("reason", msg))
	_ = s.repo.MarkFailed(ctx, id, msg)
}

func extractReviewJSON(s string) string {
	start := 0
	for i, ch := range s {
		if ch == '{' {
			start = i
			break
		}
	}
	end := len(s) - 1
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '}' {
			end = i
			break
		}
	}
	if start < end {
		return s[start : end+1]
	}
	return s
}
