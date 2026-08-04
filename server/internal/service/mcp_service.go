package service

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/mcp"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type MCPService interface {
	Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateMCPServerRequest) (*model.MCPServer, error)
	Get(ctx context.Context, id uuid.UUID) (*model.MCPServer, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.MCPServer, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateMCPServerRequest) (*model.MCPServer, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ListTools(ctx context.Context, id uuid.UUID) ([]mcp.Tool, error)
}

type mcpService struct {
	repo   repository.MCPRepository
	logger *zap.Logger
}

func NewMCPService(repo repository.MCPRepository, logger *zap.Logger) MCPService {
	return &mcpService{repo: repo, logger: logger}
}

func (s *mcpService) Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateMCPServerRequest) (*model.MCPServer, error) {
	transport := req.Transport
	if transport == "" {
		transport = model.MCPTransportHTTP
	}
	m := &model.MCPServer{
		OrgID:      orgID,
		Name:       req.Name,
		Transport:  transport,
		URL:        req.URL,
		AuthHeader: req.AuthHeader,
		Enabled:    true,
		CreatedBy:  &createdBy,
	}
	if err := s.repo.Create(ctx, m); err != nil {
		return nil, fmt.Errorf("create mcp server: %w", err)
	}
	return m, nil
}

func (s *mcpService) Get(ctx context.Context, id uuid.UUID) (*model.MCPServer, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("mcp server not found")
	}
	return m, nil
}

func (s *mcpService) List(ctx context.Context, orgID uuid.UUID) ([]model.MCPServer, error) {
	return s.repo.ListByOrg(ctx, orgID)
}

func (s *mcpService) Update(ctx context.Context, id uuid.UUID, req model.UpdateMCPServerRequest) (*model.MCPServer, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil || m == nil {
		return nil, fmt.Errorf("mcp server not found")
	}

	if req.Name != nil {
		m.Name = *req.Name
	}
	if req.URL != nil {
		m.URL = *req.URL
	}
	if req.AuthHeader != nil {
		m.AuthHeader = *req.AuthHeader
	}
	if req.Enabled != nil {
		m.Enabled = *req.Enabled
	}

	if err := s.repo.Update(ctx, m); err != nil {
		return nil, err
	}
	return m, nil
}

func (s *mcpService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *mcpService) ListTools(ctx context.Context, id uuid.UUID) ([]mcp.Tool, error) {
	m, err := s.repo.FindByID(ctx, id)
	if err != nil || m == nil {
		return nil, fmt.Errorf("mcp server not found")
	}

	client := mcp.NewClient(m.URL, m.AuthHeader)
	if err := client.Initialize(ctx); err != nil {
		return nil, fmt.Errorf("initialize mcp server %q: %w", m.Name, err)
	}
	tools, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("list tools from %q: %w", m.Name, err)
	}
	return tools, nil
}
