package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/engine"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// PluginService handles CRUD and execution of plugins.
type PluginService interface {
	Create(ctx context.Context, orgID uuid.UUID, createdBy uuid.UUID, req model.CreatePluginRequest) (*model.Plugin, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Plugin, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Plugin], error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdatePluginRequest) (*model.Plugin, error)
	Delete(ctx context.Context, id uuid.UUID) error
	Execute(ctx context.Context, pluginID uuid.UUID, orgID uuid.UUID, req model.ExecutePluginRequest) (*model.PluginExecution, error)
	ListExecutions(ctx context.Context, pluginID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.PluginExecution], error)
}

type pluginService struct {
	pluginRepo   repository.PluginRepository
	agentRepo    repository.AgentRepository
	engineRouter *engine.Router
	logger       *zap.Logger
}

func NewPluginService(
	pluginRepo repository.PluginRepository,
	agentRepo repository.AgentRepository,
	engineRouter *engine.Router,
	logger *zap.Logger,
) PluginService {
	return &pluginService{
		pluginRepo:   pluginRepo,
		agentRepo:    agentRepo,
		engineRouter: engineRouter,
		logger:       logger,
	}
}

func (s *pluginService) Create(ctx context.Context, orgID uuid.UUID, createdBy uuid.UUID, req model.CreatePluginRequest) (*model.Plugin, error) {
	p := &model.Plugin{
		ID:          uuid.New(),
		OrgID:       orgID,
		Name:        req.Name,
		Version:     req.Version,
		Description: req.Description,
		PluginType:  req.PluginType,
		WorkflowDef: req.WorkflowDef,
		Permissions: req.Permissions,
		Config:      req.Config,
		CreatedBy:   &createdBy,
	}
	if p.Version == "" {
		p.Version = "1.0.0"
	}
	if p.PluginType == "" {
		p.PluginType = "langgraph"
	}
	if p.Permissions == nil {
		p.Permissions = []string{"llm_access"}
	}
	if p.Config == nil {
		p.Config = map[string]any{}
	}

	if err := s.pluginRepo.Create(ctx, p); err != nil {
		return nil, fmt.Errorf("plugin_svc: create: %w", err)
	}
	return p, nil
}

func (s *pluginService) GetByID(ctx context.Context, id uuid.UUID) (*model.Plugin, error) {
	p, err := s.pluginRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("plugin_svc: get: %w", err)
	}
	return p, nil
}

func (s *pluginService) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Plugin], error) {
	return s.pluginRepo.ListByOrg(ctx, orgID, params)
}

func (s *pluginService) Update(ctx context.Context, id uuid.UUID, req model.UpdatePluginRequest) (*model.Plugin, error) {
	p, err := s.pluginRepo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("plugin_svc: find for update: %w", err)
	}
	if p == nil {
		return nil, ErrAgentNotFound
	}

	if req.Name != nil {
		p.Name = *req.Name
	}
	if req.Version != nil {
		p.Version = *req.Version
	}
	if req.Description != nil {
		p.Description = req.Description
	}
	if req.Status != nil {
		p.Status = *req.Status
	}
	if req.WorkflowDef != nil {
		p.WorkflowDef = req.WorkflowDef
	}
	if req.Permissions != nil {
		p.Permissions = req.Permissions
	}
	if req.Config != nil {
		p.Config = req.Config
	}

	if err := s.pluginRepo.Update(ctx, p); err != nil {
		return nil, fmt.Errorf("plugin_svc: update: %w", err)
	}
	return p, nil
}

func (s *pluginService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.pluginRepo.Delete(ctx, id)
}

func (s *pluginService) Execute(ctx context.Context, pluginID uuid.UUID, orgID uuid.UUID, req model.ExecutePluginRequest) (*model.PluginExecution, error) {
	p, err := s.pluginRepo.FindByID(ctx, pluginID)
	if err != nil || p == nil {
		return nil, fmt.Errorf("plugin_svc: plugin not found")
	}

	if p.Status != model.PluginStatusActive {
		return nil, fmt.Errorf("plugin_svc: plugin is not active (status=%s)", p.Status)
	}

	// Validate permissions — for now, just log.
	s.logger.Info("executing plugin",
		zap.String("plugin_id", pluginID.String()),
		zap.String("plugin_name", p.Name),
		zap.Strings("permissions", p.Permissions),
	)

	pe := &model.PluginExecution{
		ID:       uuid.New(),
		PluginID: pluginID,
		OrgID:    orgID,
		Input:    req.Input,
		Status:   model.ExecutionStatusPending,
	}
	if err := s.pluginRepo.CreateExecution(ctx, pe); err != nil {
		return nil, fmt.Errorf("plugin_svc: create execution: %w", err)
	}

	// Mark as running.
	now := time.Now()
	pe.Status = model.ExecutionStatusRunning
	pe.StartedAt = &now
	_ = s.pluginRepo.UpdateExecution(ctx, pe)

	// Build a synthetic agent from the plugin to run through the engine.
	engineType := model.EngineLangGraph
	if p.PluginType == "langchain" {
		engineType = model.EngineLangChain
	}

	// Build the prompt from the input.
	prompt := fmt.Sprintf("Execute plugin '%s' with input: %v", p.Name, req.Input)
	if inputPrompt, ok := req.Input["prompt"].(string); ok {
		prompt = inputPrompt
	}

	syntheticAgent := &model.Agent{
		ID:           pluginID, // reuse plugin ID for correlation
		OrgID:        orgID,
		Name:         p.Name,
		Role:         "plugin",
		EngineType:   engineType,
		EngineConfig: map[string]any{"graph_definition": p.WorkflowDef},
	}

	execID := uuid.New()
	pe.ExecutionID = &execID

	runner := s.engineRouter.For(engineType)
	result := runner.Run(ctx, execID, syntheticAgent, prompt, []string{})

	completedAt := time.Now()
	pe.CompletedAt = &completedAt

	if result.Err != nil {
		errMsg := result.Err.Error()
		pe.Status = model.ExecutionStatusFailed
		pe.ErrorMessage = &errMsg
	} else {
		pe.Status = model.ExecutionStatusCompleted
		pe.Output = &result.FinalAnswer
	}

	_ = s.pluginRepo.UpdateExecution(ctx, pe)
	return pe, nil
}

func (s *pluginService) ListExecutions(ctx context.Context, pluginID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.PluginExecution], error) {
	return s.pluginRepo.ListExecutions(ctx, pluginID, params)
}
