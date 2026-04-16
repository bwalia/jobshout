package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/jobshout/server/internal/costengine"
	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
)

// ─── Mock Repositories ──────────────────────────────────────────────────────

type mockBudgetRepo struct {
	budgets []model.OrgBudget
	alerts  []model.BudgetAlert
}

func (m *mockBudgetRepo) Upsert(_ context.Context, b *model.OrgBudget) (*model.OrgBudget, error) {
	b.ID = uuid.New()
	m.budgets = append(m.budgets, *b)
	return b, nil
}
func (m *mockBudgetRepo) GetByOrgAndPeriod(_ context.Context, _ uuid.UUID, _ string) (*model.OrgBudget, error) {
	if len(m.budgets) > 0 {
		return &m.budgets[0], nil
	}
	return nil, nil
}
func (m *mockBudgetRepo) List(_ context.Context, _ uuid.UUID) ([]model.OrgBudget, error) {
	return m.budgets, nil
}
func (m *mockBudgetRepo) Delete(_ context.Context, _ uuid.UUID) error { return nil }
func (m *mockBudgetRepo) RecordAlert(_ context.Context, a *model.BudgetAlert) error {
	m.alerts = append(m.alerts, *a)
	return nil
}
func (m *mockBudgetRepo) ListAlerts(_ context.Context, _ uuid.UUID, _ int) ([]model.BudgetAlert, error) {
	return m.alerts, nil
}

type mockPolicyRepo struct {
	agentPolicy *model.AgentPolicy
	orgPolicy   *model.AgentPolicy
	policies    []model.AgentPolicy
}

func (m *mockPolicyRepo) Upsert(_ context.Context, p *model.AgentPolicy) (*model.AgentPolicy, error) {
	p.ID = uuid.New()
	return p, nil
}
func (m *mockPolicyRepo) GetForAgent(_ context.Context, _, _ uuid.UUID) (*model.AgentPolicy, error) {
	return m.agentPolicy, nil
}
func (m *mockPolicyRepo) GetOrgDefault(_ context.Context, _ uuid.UUID) (*model.AgentPolicy, error) {
	return m.orgPolicy, nil
}
func (m *mockPolicyRepo) List(_ context.Context, _ uuid.UUID) ([]model.AgentPolicy, error) {
	return m.policies, nil
}
func (m *mockPolicyRepo) Delete(_ context.Context, _ uuid.UUID) error { return nil }

type mockUsageRepo struct {
	recentCount int
	periodSpend float64
}

func (m *mockUsageRepo) UpsertRollup(_ context.Context, _ *model.UsageRollup) error { return nil }
func (m *mockUsageRepo) QueryRollups(_ context.Context, _ model.UsageQueryParams) ([]model.UsageRollup, error) {
	return nil, nil
}
func (m *mockUsageRepo) OrgSpendSummary(_ context.Context, _ uuid.UUID, _, _ time.Time) (*model.OrgUsageSummary, error) {
	return nil, nil
}
func (m *mockUsageRepo) AgentAnalytics(_ context.Context, _ uuid.UUID, _, _ time.Time) (*model.AgentAnalytics, error) {
	return nil, nil
}
func (m *mockUsageRepo) TopAgentsBySpend(_ context.Context, _ uuid.UUID, _ int, _, _ time.Time) ([]model.AgentAnalytics, error) {
	return nil, nil
}
func (m *mockUsageRepo) CurrentPeriodSpend(_ context.Context, _ uuid.UUID, _ string) (float64, error) {
	return m.periodSpend, nil
}
func (m *mockUsageRepo) RecentExecCount(_ context.Context, _ uuid.UUID, _ time.Duration) (int, error) {
	return m.recentCount, nil
}

type mockExecRepo struct{}

func (m *mockExecRepo) Create(_ context.Context, _ *model.AgentExecution) error     { return nil }
func (m *mockExecRepo) MarkStarted(_ context.Context, _ uuid.UUID) error            { return nil }
func (m *mockExecRepo) MarkCompleted(_ context.Context, _ uuid.UUID, _ string, _ int, _ int) error {
	return nil
}
func (m *mockExecRepo) MarkFailed(_ context.Context, _ uuid.UUID, _ string, _ int, _ int) error {
	return nil
}
func (m *mockExecRepo) RecordToolCall(_ context.Context, _ *model.ExecutionToolCall) error {
	return nil
}
func (m *mockExecRepo) GetByID(_ context.Context, _ uuid.UUID) (*model.AgentExecution, error) {
	return nil, nil
}
func (m *mockExecRepo) ListByAgent(_ context.Context, _ uuid.UUID, _ model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error) {
	return nil, nil
}

func (m *mockExecRepo) PersistResult(_ context.Context, _ uuid.UUID, _ executor.Result) error {
	return nil
}
func (m *mockExecRepo) UpdateCostFields(_ context.Context, _ uuid.UUID, _, _, _ int, _ float64, _, _ string) error {
	return nil
}
func (m *mockExecRepo) RecordLangChainTrace(_ context.Context, _ *model.LangChainRunTrace) error {
	return nil
}
func (m *mockExecRepo) ListLangChainTraces(_ context.Context, _ uuid.UUID) ([]model.LangChainRunTrace, error) {
	return nil, nil
}
func (m *mockExecRepo) RecordLangGraphSnapshot(_ context.Context, _ *model.LangGraphStateSnapshot) error {
	return nil
}
func (m *mockExecRepo) ListLangGraphSnapshots(_ context.Context, _ uuid.UUID) ([]model.LangGraphStateSnapshot, error) {
	return nil, nil
}

// ─── Tests ──────────────────────────────────────────────────────────────────

func newTestService(policyRepo *mockPolicyRepo, budgetRepo *mockBudgetRepo, usageRepo *mockUsageRepo) GovernanceService {
	return NewGovernanceService(budgetRepo, policyRepo, usageRepo, &mockExecRepo{}, costengine.New(), nil)
}

func TestEnforcePolicy_NoPolicyAllowsAll(t *testing.T) {
	svc := newTestService(&mockPolicyRepo{}, &mockBudgetRepo{}, &mockUsageRepo{})

	err := svc.EnforcePolicy(context.Background(), uuid.New(), uuid.New(), "openai", "gpt-4o")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestEnforcePolicy_BlocksDisallowedProvider(t *testing.T) {
	svc := newTestService(
		&mockPolicyRepo{
			orgPolicy: &model.AgentPolicy{
				Enabled:          true,
				AllowedProviders: []string{"ollama"},
			},
		},
		&mockBudgetRepo{},
		&mockUsageRepo{},
	)

	err := svc.EnforcePolicy(context.Background(), uuid.New(), uuid.New(), "openai", "gpt-4o")
	if err == nil {
		t.Fatal("expected policy blocked error")
	}
	if !errors.Is(err, ErrPolicyBlocked) {
		t.Fatalf("expected ErrPolicyBlocked, got %v", err)
	}
}

func TestEnforcePolicy_BlocksDisallowedModel(t *testing.T) {
	svc := newTestService(
		&mockPolicyRepo{
			orgPolicy: &model.AgentPolicy{
				Enabled:       true,
				AllowedModels: []string{"llama3", "gpt-4o-mini"},
			},
		},
		&mockBudgetRepo{},
		&mockUsageRepo{},
	)

	err := svc.EnforcePolicy(context.Background(), uuid.New(), uuid.New(), "openai", "gpt-4o")
	if !errors.Is(err, ErrPolicyBlocked) {
		t.Fatalf("expected ErrPolicyBlocked for model, got %v", err)
	}
}

func TestEnforcePolicy_BlocksHourlyRateLimit(t *testing.T) {
	maxPerHour := 5
	svc := newTestService(
		&mockPolicyRepo{
			orgPolicy: &model.AgentPolicy{
				Enabled:         true,
				MaxExecsPerHour: &maxPerHour,
			},
		},
		&mockBudgetRepo{},
		&mockUsageRepo{recentCount: 5},
	)

	err := svc.EnforcePolicy(context.Background(), uuid.New(), uuid.New(), "", "")
	if !errors.Is(err, ErrPolicyBlocked) {
		t.Fatalf("expected ErrPolicyBlocked for hourly limit, got %v", err)
	}
}

func TestEnforcePolicy_BlocksHardBudgetLimit(t *testing.T) {
	hardLimit := 100.0
	svc := newTestService(
		&mockPolicyRepo{},
		&mockBudgetRepo{
			budgets: []model.OrgBudget{{
				ID:           uuid.New(),
				Period:       "monthly",
				HardLimitUSD: &hardLimit,
				Enabled:      true,
			}},
		},
		&mockUsageRepo{periodSpend: 100.0},
	)

	err := svc.EnforcePolicy(context.Background(), uuid.New(), uuid.New(), "", "")
	if !errors.Is(err, ErrBudgetExceeded) {
		t.Fatalf("expected ErrBudgetExceeded, got %v", err)
	}
}

func TestEnforcePolicy_AllowsUnderBudget(t *testing.T) {
	hardLimit := 100.0
	svc := newTestService(
		&mockPolicyRepo{},
		&mockBudgetRepo{
			budgets: []model.OrgBudget{{
				ID:           uuid.New(),
				Period:       "monthly",
				HardLimitUSD: &hardLimit,
				Enabled:      true,
			}},
		},
		&mockUsageRepo{periodSpend: 50.0},
	)

	err := svc.EnforcePolicy(context.Background(), uuid.New(), uuid.New(), "", "")
	if err != nil {
		t.Fatalf("expected no error (under budget), got %v", err)
	}
}

func TestEnforcePolicy_AgentPolicyTakesPrecedence(t *testing.T) {
	svc := newTestService(
		&mockPolicyRepo{
			agentPolicy: &model.AgentPolicy{
				Enabled:          true,
				AllowedProviders: []string{"openai"}, // agent-specific allows openai
			},
			orgPolicy: &model.AgentPolicy{
				Enabled:          true,
				AllowedProviders: []string{"ollama"}, // org default only allows ollama
			},
		},
		&mockBudgetRepo{},
		&mockUsageRepo{},
	)

	// Agent-specific policy allows openai, so this should pass.
	err := svc.EnforcePolicy(context.Background(), uuid.New(), uuid.New(), "openai", "gpt-4o")
	if err != nil {
		t.Fatalf("expected agent policy to take precedence, got %v", err)
	}
}
