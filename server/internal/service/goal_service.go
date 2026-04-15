package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// GoalService manages the lifecycle of autonomous agent goals.
type GoalService interface {
	CreateGoal(ctx context.Context, orgID, agentID uuid.UUID, req model.CreateGoalRequest) (*model.AgentGoal, error)
	GetGoal(ctx context.Context, id uuid.UUID) (*model.AgentGoal, error)
	ListGoals(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentGoal], error)
}

type goalService struct {
	goalRepo     repository.GoalRepository
	agentRepo    repository.AgentRepository
	autoExec     *executor.AutonomousExecutor
	toolPermRepo repository.AgentToolRepository
	logger       *zap.Logger
}

func NewGoalService(
	goalRepo repository.GoalRepository,
	agentRepo repository.AgentRepository,
	toolPermRepo repository.AgentToolRepository,
	autoExec *executor.AutonomousExecutor,
	logger *zap.Logger,
) GoalService {
	return &goalService{
		goalRepo:     goalRepo,
		agentRepo:    agentRepo,
		toolPermRepo: toolPermRepo,
		autoExec:     autoExec,
		logger:       logger,
	}
}

func (s *goalService) CreateGoal(ctx context.Context, orgID, agentID uuid.UUID, req model.CreateGoalRequest) (*model.AgentGoal, error) {
	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("goal_svc: agent not found: %w", err)
	}

	maxIter := req.MaxIter
	if maxIter <= 0 {
		maxIter = executor.MaxGoalIterations
	}

	goal := &model.AgentGoal{
		ID:        uuid.New(),
		AgentID:   agentID,
		OrgID:     orgID,
		SessionID: req.SessionID,
		GoalText:  req.GoalText,
		Plan:      []model.PlanStep{},
		Status:    model.GoalStatusPending,
		MaxIter:   maxIter,
	}

	if err := s.goalRepo.Create(ctx, goal); err != nil {
		return nil, fmt.Errorf("goal_svc: create: %w", err)
	}

	// Load agent tools for execution.
	agentTools, _ := s.toolPermRepo.ListByAgent(ctx, agentID)

	// Run the autonomous loop asynchronously.
	go func() {
		bgCtx := context.Background()
		log := s.logger.With(zap.String("goal_id", goal.ID.String()))
		log.Info("starting autonomous goal execution")

		result := s.autoExec.RunGoal(bgCtx, goal.ID, agent, agentTools)
		if result.Err != nil {
			log.Error("autonomous goal failed", zap.Error(result.Err))
		} else {
			log.Info("autonomous goal completed",
				zap.Int("iterations", result.Iterations),
				zap.String("reflection", result.Reflection),
			)
		}
	}()

	return goal, nil
}

func (s *goalService) GetGoal(ctx context.Context, id uuid.UUID) (*model.AgentGoal, error) {
	return s.goalRepo.GetByID(ctx, id)
}

func (s *goalService) ListGoals(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentGoal], error) {
	return s.goalRepo.ListByAgent(ctx, agentID, params)
}
