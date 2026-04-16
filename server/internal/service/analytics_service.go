package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// AnalyticsService provides read-only access to usage analytics and cost data.
type AnalyticsService interface {
	OrgUsageSummary(ctx context.Context, orgID uuid.UUID, from, to time.Time) (*model.OrgUsageSummary, error)
	AgentAnalytics(ctx context.Context, agentID uuid.UUID, from, to time.Time) (*model.AgentAnalytics, error)
	UsageTimeSeries(ctx context.Context, params model.UsageQueryParams) ([]model.UsageRollup, error)
	TopAgentsBySpend(ctx context.Context, orgID uuid.UUID, limit int, from, to time.Time) ([]model.AgentAnalytics, error)
}

type analyticsService struct {
	usageRepo repository.UsageRepository
	logger    *zap.Logger
}

// NewAnalyticsService creates an AnalyticsService.
func NewAnalyticsService(usageRepo repository.UsageRepository, logger *zap.Logger) AnalyticsService {
	return &analyticsService{usageRepo: usageRepo, logger: logger}
}

func (s *analyticsService) OrgUsageSummary(ctx context.Context, orgID uuid.UUID, from, to time.Time) (*model.OrgUsageSummary, error) {
	return s.usageRepo.OrgSpendSummary(ctx, orgID, from, to)
}

func (s *analyticsService) AgentAnalytics(ctx context.Context, agentID uuid.UUID, from, to time.Time) (*model.AgentAnalytics, error) {
	return s.usageRepo.AgentAnalytics(ctx, agentID, from, to)
}

func (s *analyticsService) UsageTimeSeries(ctx context.Context, params model.UsageQueryParams) ([]model.UsageRollup, error) {
	return s.usageRepo.QueryRollups(ctx, params)
}

func (s *analyticsService) TopAgentsBySpend(ctx context.Context, orgID uuid.UUID, limit int, from, to time.Time) ([]model.AgentAnalytics, error) {
	return s.usageRepo.TopAgentsBySpend(ctx, orgID, limit, from, to)
}
