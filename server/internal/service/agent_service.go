package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type AgentService interface {
	Create(ctx context.Context, orgID uuid.UUID, createdBy uuid.UUID, req model.CreateAgentRequest) (*model.Agent, error)
	GetByID(ctx context.Context, id uuid.UUID) (*model.Agent, error)
	List(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Agent], error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateAgentRequest) (*model.Agent, error)
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

type agentService struct {
	repo   repository.AgentRepository
	logger *zap.Logger
}

func NewAgentService(repo repository.AgentRepository, logger *zap.Logger) AgentService {
	return &agentService{repo: repo, logger: logger}
}

func (s *agentService) Create(ctx context.Context, orgID uuid.UUID, createdBy uuid.UUID, req model.CreateAgentRequest) (*model.Agent, error) {
	var managerID *uuid.UUID
	if req.ManagerID != nil {
		parsed, err := uuid.Parse(*req.ManagerID)
		if err != nil {
			return nil, fmt.Errorf("invalid manager_id: %w", err)
		}
		managerID = &parsed
	}

	agent := &model.Agent{
		ID:            uuid.New(),
		OrgID:         orgID,
		Name:          req.Name,
		Role:          req.Role,
		Description:   req.Description,
		Status:        "idle",
		ModelProvider: req.ModelProvider,
		ModelName:     req.ModelName,
		SystemPrompt:  req.SystemPrompt,
		ManagerID:     managerID,
		CreatedBy:     &createdBy,
	}

	if err := s.repo.Create(ctx, agent); err != nil {
		return nil, fmt.Errorf("creating agent: %w", err)
	}
	return agent, nil
}

func (s *agentService) GetByID(ctx context.Context, id uuid.UUID) (*model.Agent, error) {
	agent, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("getting agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}
	return agent, nil
}

func (s *agentService) List(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Agent], error) {
	return s.repo.ListByOrg(ctx, orgID, params)
}

func (s *agentService) Update(ctx context.Context, id uuid.UUID, req model.UpdateAgentRequest) (*model.Agent, error) {
	agent, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("finding agent: %w", err)
	}
	if agent == nil {
		return nil, ErrAgentNotFound
	}

	if req.Name != nil {
		agent.Name = *req.Name
	}
	if req.Role != nil {
		agent.Role = *req.Role
	}
	if req.Description != nil {
		agent.Description = req.Description
	}
	if req.ModelProvider != nil {
		agent.ModelProvider = req.ModelProvider
	}
	if req.ModelName != nil {
		agent.ModelName = req.ModelName
	}
	if req.SystemPrompt != nil {
		agent.SystemPrompt = req.SystemPrompt
	}
	if req.ManagerID != nil {
		parsed, err := uuid.Parse(*req.ManagerID)
		if err != nil {
			return nil, fmt.Errorf("invalid manager_id: %w", err)
		}
		agent.ManagerID = &parsed
	}

	if err := s.repo.Update(ctx, agent); err != nil {
		return nil, fmt.Errorf("updating agent: %w", err)
	}
	return agent, nil
}

func (s *agentService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *agentService) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	return s.repo.UpdateStatus(ctx, id, status)
}

var ErrAgentNotFound = agentError("agent not found")

type agentError string

func (e agentError) Error() string { return string(e) }
