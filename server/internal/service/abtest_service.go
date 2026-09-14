package service

import (
	"context"

	"github.com/jobshout/server/internal/abtest"
	"github.com/jobshout/server/internal/wslproxymcp"
)

// ABTestService is the HTTP façade for the AB Testing agent.
type ABTestService interface {
	Enabled() bool
	Status(ctx context.Context) map[string]any
	ListExperiments(ctx context.Context) ([]abtest.Experiment, string, error)
	GetExperiment(ctx context.Context, id string) (*abtest.Experiment, error)
	SetWeights(ctx context.Context, id string, backends []wslproxymcp.BackendWeight) (map[string]any, error)
	Promote(ctx context.Context, id, label string) (map[string]any, error)
	Rollback(ctx context.Context, id string) (map[string]any, error)
	Observe(ctx context.Context, id string, n int) (map[string]any, error)
}

type abTestService struct {
	client *abtest.Client
}

func NewABTestService(client *abtest.Client) ABTestService {
	return &abTestService{client: client}
}

func (s *abTestService) Enabled() bool {
	return s != nil && s.client != nil && s.client.Enabled()
}

func (s *abTestService) Status(ctx context.Context) map[string]any {
	return s.client.Status(ctx)
}

func (s *abTestService) ListExperiments(ctx context.Context) ([]abtest.Experiment, string, error) {
	return s.client.ListExperiments(ctx)
}

func (s *abTestService) GetExperiment(ctx context.Context, id string) (*abtest.Experiment, error) {
	return s.client.GetExperiment(ctx, id)
}

func (s *abTestService) SetWeights(ctx context.Context, id string, backends []wslproxymcp.BackendWeight) (map[string]any, error) {
	return s.client.SetWeights(ctx, id, backends)
}

func (s *abTestService) Promote(ctx context.Context, id, label string) (map[string]any, error) {
	return s.client.Promote(ctx, id, label)
}

func (s *abTestService) Rollback(ctx context.Context, id string) (map[string]any, error) {
	return s.client.Rollback(ctx, id)
}

func (s *abTestService) Observe(ctx context.Context, id string, n int) (map[string]any, error) {
	return s.client.Observe(ctx, id, n)
}
