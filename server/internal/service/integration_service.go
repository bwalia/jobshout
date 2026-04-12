package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type IntegrationService interface {
	Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateIntegrationRequest) (*model.Integration, error)
	Get(ctx context.Context, id uuid.UUID) (*model.Integration, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateIntegrationRequest) (*model.Integration, error)
	Delete(ctx context.Context, id uuid.UUID) error

	LinkTask(ctx context.Context, integrationID, taskID uuid.UUID, direction string) (*model.IntegrationTaskLink, error)
	UnlinkTask(ctx context.Context, integrationID, taskID uuid.UUID) error
	ListLinks(ctx context.Context, integrationID uuid.UUID) ([]model.IntegrationTaskLink, error)
	SyncLink(ctx context.Context, linkID uuid.UUID, direction string) error
	ListSyncLogs(ctx context.Context, integrationID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.IntegrationSyncLog], error)
}

type integrationService struct {
	integRepo   repository.IntegrationRepository
	linkRepo    repository.TaskLinkRepository
	syncLogRepo repository.SyncLogRepository
	registry    *integration.Registry
	logger      *zap.Logger
}

func NewIntegrationService(
	integRepo repository.IntegrationRepository,
	linkRepo repository.TaskLinkRepository,
	syncLogRepo repository.SyncLogRepository,
	registry *integration.Registry,
	logger *zap.Logger,
) IntegrationService {
	return &integrationService{
		integRepo:   integRepo,
		linkRepo:    linkRepo,
		syncLogRepo: syncLogRepo,
		registry:    registry,
		logger:      logger,
	}
}

func (s *integrationService) Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateIntegrationRequest) (*model.Integration, error) {
	i := &model.Integration{
		OrgID:       orgID,
		Name:        req.Name,
		Provider:    req.Provider,
		BaseURL:     req.BaseURL,
		Credentials: req.Credentials,
		Config:      req.Config,
		Status:      "active",
		CreatedBy:   &createdBy,
	}
	if i.Config == nil {
		i.Config = map[string]any{}
	}
	if i.Credentials == nil {
		i.Credentials = map[string]any{}
	}

	if err := s.integRepo.Create(ctx, i); err != nil {
		return nil, fmt.Errorf("create integration: %w", err)
	}
	return i, nil
}

func (s *integrationService) Get(ctx context.Context, id uuid.UUID) (*model.Integration, error) {
	i, err := s.integRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if i == nil {
		return nil, fmt.Errorf("integration not found")
	}
	return i, nil
}

func (s *integrationService) List(ctx context.Context, orgID uuid.UUID) ([]model.Integration, error) {
	return s.integRepo.ListByOrg(ctx, orgID)
}

func (s *integrationService) Update(ctx context.Context, id uuid.UUID, req model.UpdateIntegrationRequest) (*model.Integration, error) {
	i, err := s.integRepo.FindByID(ctx, id)
	if err != nil || i == nil {
		return nil, fmt.Errorf("integration not found")
	}

	if req.Name != nil {
		i.Name = *req.Name
	}
	if req.BaseURL != nil {
		i.BaseURL = *req.BaseURL
	}
	if req.Credentials != nil {
		i.Credentials = req.Credentials
	}
	if req.Config != nil {
		i.Config = req.Config
	}
	if req.Status != nil {
		i.Status = *req.Status
	}

	if err := s.integRepo.Update(ctx, i); err != nil {
		return nil, err
	}
	return i, nil
}

func (s *integrationService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.integRepo.Delete(ctx, id)
}

func (s *integrationService) LinkTask(ctx context.Context, integrationID, taskID uuid.UUID, direction string) (*model.IntegrationTaskLink, error) {
	if direction == "" {
		direction = "bidirectional"
	}

	integ, err := s.integRepo.FindByID(ctx, integrationID)
	if err != nil || integ == nil {
		return nil, fmt.Errorf("integration not found")
	}

	adapter, err := s.registry.GetTask(*integ)
	if err != nil {
		return nil, fmt.Errorf("get adapter: %w", err)
	}

	// Create the external issue
	issue := integration.ExternalIssue{
		Title:       fmt.Sprintf("Jobshout Task %s", taskID.String()[:8]),
		Description: "Linked from Jobshout",
	}

	start := time.Now()
	externalID, externalURL, err := adapter.CreateIssue(ctx, issue)
	duration := int(time.Since(start).Milliseconds())

	// Log the sync operation
	logStatus := "success"
	var errMsg *string
	if err != nil {
		logStatus = "failed"
		e := err.Error()
		errMsg = &e
	}
	_ = s.syncLogRepo.Append(ctx, &model.IntegrationSyncLog{
		IntegrationID: integrationID,
		Direction:     "push",
		Status:        logStatus,
		ErrorMessage:  errMsg,
		DurationMs:    &duration,
	})

	if err != nil {
		return nil, fmt.Errorf("create external issue: %w", err)
	}

	link := &model.IntegrationTaskLink{
		IntegrationID: integrationID,
		TaskID:        taskID,
		ExternalID:    externalID,
		ExternalURL:   &externalURL,
		SyncDirection: direction,
		SyncStatus:    "synced",
	}
	now := time.Now()
	link.LastSyncedAt = &now

	if err := s.linkRepo.Create(ctx, link); err != nil {
		return nil, fmt.Errorf("create link: %w", err)
	}
	return link, nil
}

func (s *integrationService) UnlinkTask(ctx context.Context, integrationID, taskID uuid.UUID) error {
	return s.linkRepo.DeleteByTaskAndIntegration(ctx, integrationID, taskID)
}

func (s *integrationService) ListLinks(ctx context.Context, integrationID uuid.UUID) ([]model.IntegrationTaskLink, error) {
	return s.linkRepo.ListByIntegration(ctx, integrationID)
}

func (s *integrationService) SyncLink(ctx context.Context, linkID uuid.UUID, direction string) error {
	// This would be expanded with the full sync engine
	s.logger.Info("sync link requested", zap.String("link_id", linkID.String()), zap.String("direction", direction))
	return nil
}

func (s *integrationService) ListSyncLogs(ctx context.Context, integrationID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.IntegrationSyncLog], error) {
	return s.syncLogRepo.ListByIntegration(ctx, integrationID, params)
}
