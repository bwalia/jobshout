package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	integ "github.com/jobshout/server/internal/integration"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

type NotificationService interface {
	Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateNotificationConfigRequest) (*model.NotificationConfig, error)
	Get(ctx context.Context, id uuid.UUID) (*model.NotificationConfig, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.NotificationConfig, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateNotificationConfigRequest) (*model.NotificationConfig, error)
	Delete(ctx context.Context, id uuid.UUID) error
	TestConfig(ctx context.Context, id uuid.UUID) error
	DispatchEvent(ctx context.Context, orgID uuid.UUID, event integ.TaskEvent) error
	StartSubscriber(ctx context.Context, bus *integ.Bus)
}

type notificationService struct {
	repo     repository.NotificationConfigRepository
	registry *integ.Registry
	logger   *zap.Logger
}

func NewNotificationService(
	repo repository.NotificationConfigRepository,
	registry *integ.Registry,
	logger *zap.Logger,
) NotificationService {
	return &notificationService{repo: repo, registry: registry, logger: logger}
}

func (s *notificationService) Create(ctx context.Context, orgID, createdBy uuid.UUID, req model.CreateNotificationConfigRequest) (*model.NotificationConfig, error) {
	cfg := &model.NotificationConfig{
		OrgID:       orgID,
		Name:        req.Name,
		ChannelType: req.ChannelType,
		WebhookURL:  req.WebhookURL,
		Config:      req.Config,
		Enabled:     true,
		Events:      req.Events,
		CreatedBy:   &createdBy,
	}
	if cfg.Config == nil {
		cfg.Config = map[string]any{}
	}

	if err := s.repo.Create(ctx, cfg); err != nil {
		return nil, fmt.Errorf("create notification config: %w", err)
	}
	return cfg, nil
}

func (s *notificationService) Get(ctx context.Context, id uuid.UUID) (*model.NotificationConfig, error) {
	cfg, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("notification config not found")
	}
	return cfg, nil
}

func (s *notificationService) List(ctx context.Context, orgID uuid.UUID) ([]model.NotificationConfig, error) {
	return s.repo.ListByOrg(ctx, orgID)
}

func (s *notificationService) Update(ctx context.Context, id uuid.UUID, req model.UpdateNotificationConfigRequest) (*model.NotificationConfig, error) {
	cfg, err := s.repo.FindByID(ctx, id)
	if err != nil || cfg == nil {
		return nil, fmt.Errorf("notification config not found")
	}

	if req.Name != nil {
		cfg.Name = *req.Name
	}
	if req.WebhookURL != nil {
		cfg.WebhookURL = *req.WebhookURL
	}
	if req.Config != nil {
		cfg.Config = req.Config
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.Events != nil {
		cfg.Events = req.Events
	}

	if err := s.repo.Update(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (s *notificationService) Delete(ctx context.Context, id uuid.UUID) error {
	return s.repo.Delete(ctx, id)
}

func (s *notificationService) TestConfig(ctx context.Context, id uuid.UUID) error {
	cfg, err := s.repo.FindByID(ctx, id)
	if err != nil || cfg == nil {
		return fmt.Errorf("notification config not found")
	}

	adapter, err := s.registry.GetNotification(*cfg)
	if err != nil {
		return fmt.Errorf("get adapter: %w", err)
	}

	return adapter.Test(ctx)
}

func (s *notificationService) DispatchEvent(ctx context.Context, orgID uuid.UUID, event integ.TaskEvent) error {
	configs, err := s.repo.ListByOrgAndEvent(ctx, orgID, string(event.Type))
	if err != nil {
		return fmt.Errorf("list notification configs: %w", err)
	}

	msg := integ.NotificationMessage{
		OrgID:     orgID,
		EventType: string(event.Type),
		TaskTitle: event.Title,
		TaskID:    event.TaskID.String(),
		Status:    event.Status,
	}

	for _, cfg := range configs {
		adapter, err := s.registry.GetNotification(cfg)
		if err != nil {
			s.logger.Warn("skip notification config", zap.Error(err), zap.String("config_id", cfg.ID.String()))
			continue
		}

		// Fire in background with timeout
		go func(a integ.NotificationAdapter, m integ.NotificationMessage) {
			sendCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := a.Send(sendCtx, m); err != nil {
				s.logger.Warn("notification send failed", zap.Error(err), zap.String("adapter", a.Name()))
			}
		}(adapter, msg)
	}

	return nil
}

// StartSubscriber subscribes to all task events and dispatches notifications.
func (s *notificationService) StartSubscriber(ctx context.Context, bus *integ.Bus) {
	for _, eventType := range integ.AllEventTypes() {
		ch := bus.Subscribe(eventType)
		go func(ch <-chan integ.TaskEvent) {
			for {
				select {
				case <-ctx.Done():
					return
				case event := <-ch:
					if err := s.DispatchEvent(ctx, event.OrgID, event); err != nil {
						s.logger.Warn("dispatch event failed", zap.Error(err))
					}
				}
			}
		}(ch)
	}
}
