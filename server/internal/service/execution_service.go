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

	// MarkResuming moves an execution paused for approval back to running as
	// its resume starts. It reports false when the execution is no longer
	// paused (cancelled or finished) and must not be resumed.
	MarkResuming(ctx context.Context, execID uuid.UUID) (bool, error)
	// CompleteResumed persists the outcome of a resumed run exactly as a fresh
	// run's outcome is persisted (status, output, metering, usage and cost) and
	// returns the stored execution. A run that paused again is stored as
	// awaiting_approval with PendingApprovalID set.
	CompleteResumed(ctx context.Context, execID uuid.UUID, result executor.Result) (*model.AgentExecution, error)
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
	// maxTokens is the resolved agent_policies.max_tokens_per_exec for this run,
	// or 0 for no cap. Resolved once here, next to the policy check, rather than
	// looked up again at run time.
	maxTokens int
	// maxCost is the resolved agent_policies.max_cost_per_exec in USD, or 0 for
	// no cap. Resolved alongside maxTokens for the same reason.
	maxCost float64
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

	maxTokens := 0
	maxCost := 0.0
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
		maxTokens = s.govSvc.MaxTokensPerExec(ctx, orgID, agentID)
		maxCost = s.govSvc.MaxCostPerExec(ctx, orgID, agentID)
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
		maxTokens:  maxTokens,
		maxCost:    maxCost,
	}, nil
}

func (s *executionService) finish(ctx context.Context, st *startedExec) (*model.AgentExecution, error) {
	if len(st.req.SkillSlugs) > 0 || st.maxTokens > 0 || st.maxCost > 0 {
		ctx = executor.WithRunOptions(ctx, executor.RunOptions{
			SkillSlugs: st.req.SkillSlugs,
			MaxTokens:  st.maxTokens,
			MaxCostUSD: st.maxCost,
		})
	}
	runner := s.engineRouter.For(st.engineType)
	runStart := time.Now()
	result := runner.Run(ctx, st.exec.ID, st.agent, st.req.Prompt, st.agentTools)
	// Not every engine measures itself (the LangChain and LangGraph runners
	// leave LatencyMs at zero), so the wall-clock time of the run stands in.
	if result.LatencyMs <= 0 {
		result.LatencyMs = int(time.Since(runStart).Milliseconds())
	}

	return s.persistOutcome(ctx, st.exec, result)
}

// persistOutcome stores a run's result — fresh or resumed — and returns the
// execution as stored. base carries the execution's identity (ID, agent, org)
// and is the fallback returned if the stored row cannot be re-read.
//
// Usage (cost, token split, latency, rollups) is recorded synchronously once
// the run is terminal, so the returned execution — and the task run that
// mirrors it — carries the cost. A run paused for approval records no usage
// yet: its resume reports the cumulative figures once, when it ends.
func (s *executionService) persistOutcome(ctx context.Context, base *model.AgentExecution, result executor.Result) (*model.AgentExecution, error) {
	paused := result.Status == executor.StatusAwaitingApproval && result.Err == nil

	_ = s.agentRepo.UpdateStatus(ctx, base.AgentID, "idle")

	if err := s.execRepo.PersistResult(ctx, base.ID, result); err != nil {
		s.logger.Error("failed to persist execution result", zap.Error(err))
	}

	costUSD := 0.0
	if s.govSvc != nil && !paused {
		usageExec := &model.AgentExecution{
			ID:           base.ID,
			AgentID:      base.AgentID,
			OrgID:        base.OrgID,
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
		// Detached from the caller's cancellation: the run already happened
		// and its spend must be recorded even if the caller has gone.
		if err := s.govSvc.RecordUsage(context.WithoutCancel(ctx), usageExec); err != nil {
			s.logger.Error("failed to record usage", zap.Error(err))
		}
		costUSD = usageExec.CostUSD
	}

	stored, err := s.execRepo.GetByID(ctx, base.ID)
	if err == nil && stored != nil {
		// The measured values are already in hand; prefer them over a row
		// whose usage columns were not (or not yet) written.
		if stored.LatencyMs == 0 {
			stored.LatencyMs = result.LatencyMs
		}
		if stored.InputTokens == 0 && stored.OutputTokens == 0 {
			stored.InputTokens, stored.OutputTokens = result.InputTokens, result.OutputTokens
		}
		if stored.CostUSD == 0 {
			stored.CostUSD = costUSD
		}
		if paused {
			stored.Status = model.ExecutionStatusAwaitingApproval
			id := result.ApprovalID
			stored.PendingApprovalID = &id
		}
		return stored, nil
	}

	out := *base
	completedAt := time.Now()
	switch {
	case paused:
		out.Status = model.ExecutionStatusAwaitingApproval
		id := result.ApprovalID
		out.PendingApprovalID = &id
	case result.Err != nil:
		errMsg := result.Err.Error()
		out.Status = model.ExecutionStatusFailed
		out.ErrorMessage = &errMsg
		out.CompletedAt = &completedAt
	default:
		out.Status = model.ExecutionStatusCompleted
		out.Output = &result.FinalAnswer
		out.CompletedAt = &completedAt
	}
	out.TotalTokens = result.TotalTokens
	out.InputTokens = result.InputTokens
	out.OutputTokens = result.OutputTokens
	out.LatencyMs = result.LatencyMs
	out.Iterations = result.Iterations
	out.CostUSD = costUSD
	return &out, nil
}

func (s *executionService) MarkResuming(ctx context.Context, execID uuid.UUID) (bool, error) {
	return s.execRepo.MarkResuming(ctx, execID)
}

func (s *executionService) CompleteResumed(ctx context.Context, execID uuid.UUID, result executor.Result) (*model.AgentExecution, error) {
	exec, err := s.execRepo.GetByID(ctx, execID)
	if err != nil {
		return nil, fmt.Errorf("execution_svc: complete resumed: %w", err)
	}
	if exec == nil {
		return nil, ErrExecutionNotFound
	}
	return s.persistOutcome(ctx, exec, result)
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
