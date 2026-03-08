package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/llm"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
	"github.com/jobshout/server/internal/tools"
)

// ExecutionService orchestrates agent task execution end-to-end.
// It creates the AgentExecution record, drives the executor.Executor, and
// persists the result with all tool call records.
type ExecutionService interface {
	// Execute runs an agent against a prompt and returns the completed execution.
	Execute(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, prompt string) (*model.AgentExecution, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.AgentExecution, error)
	ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error)
}

type executionService struct {
	agentRepo     repository.AgentRepository
	execRepo      repository.ExecutionRepository
	toolPermRepo  repository.AgentToolRepository
	exec          *executor.Executor
	logger        *zap.Logger
}

// NewExecutionService creates an ExecutionService.
func NewExecutionService(
	agentRepo repository.AgentRepository,
	execRepo repository.ExecutionRepository,
	toolPermRepo repository.AgentToolRepository,
	llmRouter *llm.Router,
	toolRegistry *tools.Registry,
	logger *zap.Logger,
) ExecutionService {
	return &executionService{
		agentRepo:    agentRepo,
		execRepo:     execRepo,
		toolPermRepo: toolPermRepo,
		exec:         executor.New(llmRouter, toolRegistry, logger),
		logger:       logger,
	}
}

func (s *executionService) Execute(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, prompt string) (*model.AgentExecution, error) {
	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("execution_svc: find agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}

	// Resolve which tools this agent is allowed to use.
	agentTools, err := s.toolPermRepo.ListByAgent(ctx, agentID)
	if err != nil {
		s.logger.Warn("failed to load agent tool permissions; running without tools",
			zap.String("agent_id", agentID.String()),
			zap.Error(err),
		)
		agentTools = []string{}
	}

	// Create the execution record.
	execID := uuid.New()
	execRecord := &model.AgentExecution{
		ID:          execID,
		AgentID:     agentID,
		OrgID:       orgID,
		InputPrompt: prompt,
		Status:      model.ExecutionStatusPending,
	}
	if err := s.execRepo.Create(ctx, execRecord); err != nil {
		return nil, fmt.Errorf("execution_svc: create execution record: %w", err)
	}

	// Mark as running.
	if err := s.execRepo.MarkStarted(ctx, execID); err != nil {
		s.logger.Warn("failed to mark execution as started", zap.Error(err))
	}

	now := time.Now()
	execRecord.Status = model.ExecutionStatusRunning
	execRecord.StartedAt = &now

	// Update agent status to active.
	_ = s.agentRepo.UpdateStatus(ctx, agentID, "active")

	// Run the ReAct loop.
	result := s.exec.Run(ctx, execID, agent, prompt, agentTools)

	// Restore agent status.
	_ = s.agentRepo.UpdateStatus(ctx, agentID, "idle")

	// Persist the result.
	if err := s.execRepo.PersistResult(ctx, execID, result); err != nil {
		s.logger.Error("failed to persist execution result", zap.Error(err))
	}

	// Return the fully populated record.
	completed, err := s.execRepo.GetByID(ctx, execID)
	if err != nil {
		// Fallback: build the record from what we have.
		completedAt := time.Now()
		execRecord.CompletedAt = &completedAt
		if result.Err != nil {
			errMsg := result.Err.Error()
			execRecord.Status = model.ExecutionStatusFailed
			execRecord.ErrorMessage = &errMsg
		} else {
			execRecord.Status = model.ExecutionStatusCompleted
			execRecord.Output = &result.FinalAnswer
		}
		execRecord.TotalTokens = result.TotalTokens
		execRecord.Iterations = result.Iterations
		return execRecord, nil
	}
	return completed, nil
}

func (s *executionService) GetByID(ctx context.Context, id uuid.UUID) (*model.AgentExecution, error) {
	exec, err := s.execRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("execution_svc: get by id: %w", err)
	}
	return exec, nil
}

func (s *executionService) ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error) {
	return s.execRepo.ListByAgent(ctx, agentID, params)
}
