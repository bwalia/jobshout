package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/engine"
	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// ExecutionService orchestrates agent task execution end-to-end.
// It creates the AgentExecution record, drives the appropriate engine, and
// persists the result with all tool call records.
type ExecutionService interface {
	// Execute runs an agent against a prompt and returns the completed execution.
	Execute(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*model.AgentExecution, error)
	// Start creates the execution row and runs the agent in the background,
	// returning immediately with status running. Chat uses this so a 60s tool
	// timeout cannot cancel a long worker.
	Start(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*model.AgentExecution, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.AgentExecution, error)
	// Cancel stops a pending or running execution. Completed/failed runs are
	// left unchanged and returned as-is so the call is idempotent.
	Cancel(ctx context.Context, orgID, id uuid.UUID) (*model.AgentExecution, error)
	ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error)
	ListLangChainTraces(ctx context.Context, executionID uuid.UUID) ([]model.LangChainRunTrace, error)
	ListLangGraphSnapshots(ctx context.Context, executionID uuid.UUID) ([]model.LangGraphStateSnapshot, error)
}

type executionService struct {
	agentRepo    repository.AgentRepository
	execRepo     repository.ExecutionRepository
	toolPermRepo repository.AgentToolRepository
	engineRouter *engine.Router
	govSvc       GovernanceService
	logger       *zap.Logger
}

// NewExecutionService creates an ExecutionService.
// govSvc may be nil if governance is not configured.
func NewExecutionService(
	agentRepo repository.AgentRepository,
	execRepo repository.ExecutionRepository,
	toolPermRepo repository.AgentToolRepository,
	engineRouter *engine.Router,
	govSvc GovernanceService,
	logger *zap.Logger,
) ExecutionService {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &executionService{
		agentRepo:    agentRepo,
		execRepo:     execRepo,
		toolPermRepo: toolPermRepo,
		engineRouter: engineRouter,
		govSvc:       govSvc,
		logger:       logger,
	}
}

func (s *executionService) Execute(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*model.AgentExecution, error) {
	st, err := s.begin(ctx, orgID, agentID, req)
	if err != nil {
		return nil, err
	}
	return s.finish(ctx, st)
}

func (s *executionService) Start(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*model.AgentExecution, error) {
	st, err := s.begin(ctx, orgID, agentID, req)
	if err != nil {
		return nil, err
	}
	bg := context.WithoutCancel(ctx)
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("execution: background panic",
					zap.Any("panic", rec),
					zap.String("id", st.exec.ID.String()))
			}
		}()
		if _, err := s.finish(bg, st); err != nil {
			s.logger.Error("execution: background run failed",
				zap.Error(err),
				zap.String("id", st.exec.ID.String()))
		}
	}()
	return st.exec, nil
}

type startedExec struct {
	exec       *model.AgentExecution
	agent      *model.Agent
	orgID      uuid.UUID
	agentID    uuid.UUID
	req        model.ExecuteAgentRequest
	agentTools []string
	engineType string
}

func (s *executionService) begin(ctx context.Context, orgID uuid.UUID, agentID uuid.UUID, req model.ExecuteAgentRequest) (*startedExec, error) {
	agent, err := s.agentRepo.FindByID(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("execution_svc: find agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}

	if req.ModelProvider != nil && *req.ModelProvider != "" {
		clone := *agent
		clone.ModelProvider = req.ModelProvider
		if req.ModelName != nil && *req.ModelName != "" {
			clone.ModelName = req.ModelName
		}
		agent = &clone
	}

	if s.govSvc != nil {
		provider := ""
		if agent.ModelProvider != nil {
			provider = *agent.ModelProvider
		}
		modelName := ""
		if agent.ModelName != nil {
			modelName = *agent.ModelName
		}
		if err := s.govSvc.EnforcePolicy(ctx, orgID, agentID, provider, modelName); err != nil {
			return nil, err
		}
	}

	engineType := engine.ResolveEngine(agent, req.EngineOverride, "")

	agentTools, err := s.toolPermRepo.ListByAgent(ctx, agentID)
	if err != nil {
		s.logger.Warn("failed to load agent tool permissions; running without tools",
			zap.String("agent_id", agentID.String()),
			zap.Error(err),
		)
		agentTools = []string{}
	}

	execID := uuid.New()
	execRecord := &model.AgentExecution{
		ID:          execID,
		AgentID:     agentID,
		OrgID:       orgID,
		InputPrompt: req.Prompt,
		Status:      model.ExecutionStatusPending,
		EngineType:  engineType,
	}
	if err := s.execRepo.Create(ctx, execRecord); err != nil {
		return nil, fmt.Errorf("execution_svc: create execution record: %w", err)
	}

	if err := s.execRepo.MarkStarted(ctx, execID); err != nil {
		s.logger.Warn("failed to mark execution as started", zap.Error(err))
	}

	now := time.Now()
	execRecord.Status = model.ExecutionStatusRunning
	execRecord.StartedAt = &now

	_ = s.agentRepo.UpdateStatus(ctx, agentID, "active")

	return &startedExec{
		exec:       execRecord,
		agent:      agent,
		orgID:      orgID,
		agentID:    agentID,
		req:        req,
		agentTools: agentTools,
		engineType: engineType,
	}, nil
}

func (s *executionService) finish(ctx context.Context, st *startedExec) (*model.AgentExecution, error) {
	if len(st.req.SkillSlugs) > 0 {
		ctx = executor.WithRunOptions(ctx, executor.RunOptions{
			SkillSlugs: st.req.SkillSlugs,
		})
	}
	runner := s.engineRouter.For(st.engineType)
	result := runner.Run(ctx, st.exec.ID, st.agent, st.req.Prompt, st.agentTools)

	_ = s.agentRepo.UpdateStatus(ctx, st.agentID, "idle")

	if err := s.execRepo.PersistResult(ctx, st.exec.ID, result); err != nil {
		s.logger.Error("failed to persist execution result", zap.Error(err))
	}

	if s.govSvc != nil {
		usageExec := &model.AgentExecution{
			ID:           st.exec.ID,
			AgentID:      st.agentID,
			OrgID:        st.orgID,
			TotalTokens:  result.TotalTokens,
			InputTokens:  result.InputTokens,
			OutputTokens: result.OutputTokens,
			LatencyMs:    result.LatencyMs,
			Status:       model.ExecutionStatusCompleted,
		}
		if result.ModelProvider != "" {
			usageExec.ModelProvider = &result.ModelProvider
		}
		if result.ModelName != "" {
			usageExec.ModelName = &result.ModelName
		}
		if result.Err != nil {
			usageExec.Status = model.ExecutionStatusFailed
		}
		go func() {
			if err := s.govSvc.RecordUsage(context.Background(), usageExec); err != nil {
				s.logger.Error("failed to record usage", zap.Error(err))
			}
		}()
	}

	completed, err := s.execRepo.GetByID(ctx, st.exec.ID)
	if err != nil {
		completedAt := time.Now()
		st.exec.CompletedAt = &completedAt
		if result.Err != nil {
			errMsg := result.Err.Error()
			st.exec.Status = model.ExecutionStatusFailed
			st.exec.ErrorMessage = &errMsg
		} else {
			st.exec.Status = model.ExecutionStatusCompleted
			st.exec.Output = &result.FinalAnswer
		}
		st.exec.TotalTokens = result.TotalTokens
		st.exec.Iterations = result.Iterations
		return st.exec, nil
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

func (s *executionService) Cancel(ctx context.Context, orgID, id uuid.UUID) (*model.AgentExecution, error) {
	exec, err := s.execRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("execution_svc: cancel: %w", err)
	}
	if exec == nil || exec.OrgID != orgID {
		return nil, ErrExecutionNotFound
	}
	switch exec.Status {
	case model.ExecutionStatusCompleted, model.ExecutionStatusFailed, model.ExecutionStatusCancelled:
		return exec, nil
	}
	if err := s.execRepo.MarkCancelled(ctx, id); err != nil {
		return nil, err
	}
	return s.execRepo.GetByID(ctx, id)
}

func (s *executionService) ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error) {
	return s.execRepo.ListByAgent(ctx, agentID, params)
}

func (s *executionService) ListLangChainTraces(ctx context.Context, executionID uuid.UUID) ([]model.LangChainRunTrace, error) {
	return s.execRepo.ListLangChainTraces(ctx, executionID)
}

func (s *executionService) ListLangGraphSnapshots(ctx context.Context, executionID uuid.UUID) ([]model.LangGraphStateSnapshot, error) {
	return s.execRepo.ListLangGraphSnapshots(ctx, executionID)
}

var ErrExecutionNotFound = executionError("execution not found")

type executionError string

func (e executionError) Error() string { return string(e) }
