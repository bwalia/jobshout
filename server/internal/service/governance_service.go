package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/jobshout/server/internal/costengine"
	"github.com/jobshout/server/internal/model"
	"github.com/jobshout/server/internal/repository"
)

// Governance sentinel errors.
var (
	ErrPolicyBlocked  = governanceError("execution blocked by policy")
	ErrBudgetExceeded = governanceError("budget limit exceeded")
)

type governanceError string

func (e governanceError) Error() string { return string(e) }

// GovernanceService enforces policies, tracks costs, and manages budgets.
type GovernanceService interface {
	// EnforcePolicy checks all governance rules before execution. Returns ErrPolicyBlocked or ErrBudgetExceeded on denial.
	EnforcePolicy(ctx context.Context, orgID, agentID uuid.UUID, provider, modelName string) error
	// RecordUsage calculates cost and persists usage data after execution completes.
	RecordUsage(ctx context.Context, exec *model.AgentExecution) error

	// Budget CRUD
	UpsertBudget(ctx context.Context, orgID uuid.UUID, req model.CreateBudgetRequest) (*model.OrgBudget, error)
	ListBudgets(ctx context.Context, orgID uuid.UUID) ([]model.OrgBudget, error)
	DeleteBudget(ctx context.Context, budgetID uuid.UUID) error
	ListAlerts(ctx context.Context, orgID uuid.UUID) ([]model.BudgetAlert, error)

	// Policy CRUD
	UpsertPolicy(ctx context.Context, orgID uuid.UUID, req model.CreatePolicyRequest) (*model.AgentPolicy, error)
	ListPolicies(ctx context.Context, orgID uuid.UUID) ([]model.AgentPolicy, error)
	DeletePolicy(ctx context.Context, policyID uuid.UUID) error
}

type governanceService struct {
	budgetRepo repository.BudgetRepository
	policyRepo repository.PolicyRepository
	usageRepo  repository.UsageRepository
	execRepo   repository.ExecutionRepository
	costEngine *costengine.Engine
	logger     *zap.Logger
}

// NewGovernanceService creates a GovernanceService.
func NewGovernanceService(
	budgetRepo repository.BudgetRepository,
	policyRepo repository.PolicyRepository,
	usageRepo repository.UsageRepository,
	execRepo repository.ExecutionRepository,
	costEngine *costengine.Engine,
	logger *zap.Logger,
) GovernanceService {
	return &governanceService{
		budgetRepo: budgetRepo,
		policyRepo: policyRepo,
		usageRepo:  usageRepo,
		execRepo:   execRepo,
		costEngine: costEngine,
		logger:     logger,
	}
}

// ─── Policy Enforcement ─────────────────────────────────────────────────────

func (s *governanceService) EnforcePolicy(ctx context.Context, orgID, agentID uuid.UUID, provider, modelName string) error {
	// Enforce agent/org policies if any exist.
	policy := s.resolvePolicy(ctx, orgID, agentID)
	if policy != nil {
		// Check allowed providers.
		if len(policy.AllowedProviders) > 0 && provider != "" {
			if !contains(policy.AllowedProviders, provider) {
				return fmt.Errorf("%w: provider %q is not allowed", ErrPolicyBlocked, provider)
			}
		}

		// Check allowed models.
		if len(policy.AllowedModels) > 0 && modelName != "" {
			if !contains(policy.AllowedModels, modelName) {
				return fmt.Errorf("%w: model %q is not allowed", ErrPolicyBlocked, modelName)
			}
		}

		// Check hourly execution rate limit.
		if policy.MaxExecsPerHour != nil {
			count, err := s.usageRepo.RecentExecCount(ctx, agentID, time.Hour)
			if err != nil {
				s.logger.Warn("failed to check hourly exec count", zap.Error(err))
			} else if count >= *policy.MaxExecsPerHour {
				return fmt.Errorf("%w: hourly execution limit (%d) reached", ErrPolicyBlocked, *policy.MaxExecsPerHour)
			}
		}

		// Check daily execution rate limit.
		if policy.MaxExecsPerDay != nil {
			count, err := s.usageRepo.RecentExecCount(ctx, agentID, 24*time.Hour)
			if err != nil {
				s.logger.Warn("failed to check daily exec count", zap.Error(err))
			} else if count >= *policy.MaxExecsPerDay {
				return fmt.Errorf("%w: daily execution limit (%d) reached", ErrPolicyBlocked, *policy.MaxExecsPerDay)
			}
		}
	}

	// Always enforce hard budget limits regardless of policy existence.
	budgets, err := s.budgetRepo.List(ctx, orgID)
	if err != nil {
		s.logger.Warn("failed to load budgets for enforcement", zap.Error(err))
		return nil
	}
	for _, b := range budgets {
		if !b.Enabled || b.HardLimitUSD == nil {
			continue
		}
		spend, err := s.usageRepo.CurrentPeriodSpend(ctx, orgID, b.Period)
		if err != nil {
			s.logger.Warn("failed to check period spend", zap.Error(err))
			continue
		}
		if spend >= *b.HardLimitUSD {
			return fmt.Errorf("%w: %s budget $%.2f exceeded (spent $%.2f)", ErrBudgetExceeded, b.Period, *b.HardLimitUSD, spend)
		}
	}

	return nil
}

// resolvePolicy merges agent-specific + org-wide default policies. Agent-specific wins.
func (s *governanceService) resolvePolicy(ctx context.Context, orgID, agentID uuid.UUID) *model.AgentPolicy {
	agentPolicy, _ := s.policyRepo.GetForAgent(ctx, orgID, agentID)
	if agentPolicy != nil {
		return agentPolicy
	}
	orgDefault, _ := s.policyRepo.GetOrgDefault(ctx, orgID)
	return orgDefault
}

// ─── Usage Recording ────────────────────────────────────────────────────────

func (s *governanceService) RecordUsage(ctx context.Context, exec *model.AgentExecution) error {
	// Calculate cost.
	provider := ""
	if exec.ModelProvider != nil {
		provider = *exec.ModelProvider
	}
	modelName := ""
	if exec.ModelName != nil {
		modelName = *exec.ModelName
	}

	cost := s.costEngine.Calculate(provider, modelName, exec.InputTokens, exec.OutputTokens, exec.LatencyMs)
	exec.CostUSD = cost

	// Update the execution record with cost fields.
	if err := s.execRepo.UpdateCostFields(ctx, exec.ID,
		exec.InputTokens, exec.OutputTokens, exec.LatencyMs, cost, modelName, provider,
	); err != nil {
		s.logger.Error("failed to update execution cost fields", zap.Error(err))
	}

	// Upsert usage rollups (hourly + daily).
	now := time.Now().UTC()
	isError := 0
	if exec.Status == model.ExecutionStatusFailed {
		isError = 1
	}

	for _, pt := range []struct {
		periodType string
		start      time.Time
	}{
		{"hourly", time.Date(now.Year(), now.Month(), now.Day(), now.Hour(), 0, 0, 0, time.UTC)},
		{"daily", time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)},
	} {
		rollup := &model.UsageRollup{
			OrgID:         exec.OrgID,
			AgentID:       &exec.AgentID,
			ModelProvider:  provider,
			ModelName:      modelName,
			PeriodType:     pt.periodType,
			PeriodStart:    pt.start,
			ExecCount:      1,
			InputTokens:    int64(exec.InputTokens),
			OutputTokens:   int64(exec.OutputTokens),
			TotalTokens:    int64(exec.TotalTokens),
			CostUSD:        cost,
			AvgLatencyMs:   exec.LatencyMs,
			ErrorCount:     isError,
		}
		if err := s.usageRepo.UpsertRollup(ctx, rollup); err != nil {
			s.logger.Error("failed to upsert usage rollup",
				zap.String("period", pt.periodType),
				zap.Error(err),
			)
		}
	}

	// Check budget thresholds and record alerts.
	s.checkBudgetThresholds(ctx, exec.OrgID)

	return nil
}

func (s *governanceService) checkBudgetThresholds(ctx context.Context, orgID uuid.UUID) {
	budgets, err := s.budgetRepo.List(ctx, orgID)
	if err != nil {
		return
	}
	for _, b := range budgets {
		if !b.Enabled {
			continue
		}
		spend, err := s.usageRepo.CurrentPeriodSpend(ctx, orgID, b.Period)
		if err != nil {
			continue
		}

		// Check hard limit.
		if b.HardLimitUSD != nil && spend >= *b.HardLimitUSD {
			_ = s.budgetRepo.RecordAlert(ctx, &model.BudgetAlert{
				OrgID:    orgID,
				BudgetID: b.ID,
				AlertType: model.AlertTypeHardLimit,
				SpendUSD:  spend,
				LimitUSD:  *b.HardLimitUSD,
			})
		}

		// Check soft limit.
		if b.SoftLimitUSD != nil && spend >= *b.SoftLimitUSD {
			_ = s.budgetRepo.RecordAlert(ctx, &model.BudgetAlert{
				OrgID:    orgID,
				BudgetID: b.ID,
				AlertType: model.AlertTypeSoftLimit,
				SpendUSD:  spend,
				LimitUSD:  *b.SoftLimitUSD,
			})
		}

		// Check threshold percentage.
		if b.HardLimitUSD != nil && b.AlertThreshold > 0 {
			threshold := *b.HardLimitUSD * b.AlertThreshold
			if spend >= threshold {
				_ = s.budgetRepo.RecordAlert(ctx, &model.BudgetAlert{
					OrgID:    orgID,
					BudgetID: b.ID,
					AlertType: model.AlertTypeThreshold,
					SpendUSD:  spend,
					LimitUSD:  threshold,
				})
			}
		}
	}
}

// ─── Budget CRUD ────────────────────────────────────────────────────────────

func (s *governanceService) UpsertBudget(ctx context.Context, orgID uuid.UUID, req model.CreateBudgetRequest) (*model.OrgBudget, error) {
	budget := &model.OrgBudget{
		OrgID:          orgID,
		Period:         req.Period,
		SoftLimitUSD:   req.SoftLimitUSD,
		HardLimitUSD:   req.HardLimitUSD,
		AlertThreshold: 0.80,
		Enabled:        true,
	}
	if req.AlertThreshold != nil {
		budget.AlertThreshold = *req.AlertThreshold
	}
	if req.Enabled != nil {
		budget.Enabled = *req.Enabled
	}
	return s.budgetRepo.Upsert(ctx, budget)
}

func (s *governanceService) ListBudgets(ctx context.Context, orgID uuid.UUID) ([]model.OrgBudget, error) {
	return s.budgetRepo.List(ctx, orgID)
}

func (s *governanceService) DeleteBudget(ctx context.Context, budgetID uuid.UUID) error {
	return s.budgetRepo.Delete(ctx, budgetID)
}

func (s *governanceService) ListAlerts(ctx context.Context, orgID uuid.UUID) ([]model.BudgetAlert, error) {
	return s.budgetRepo.ListAlerts(ctx, orgID, 100)
}

// ─── Policy CRUD ────────────────────────────────────────────────────────────

func (s *governanceService) UpsertPolicy(ctx context.Context, orgID uuid.UUID, req model.CreatePolicyRequest) (*model.AgentPolicy, error) {
	policy := &model.AgentPolicy{
		OrgID:            orgID,
		AgentID:          req.AgentID,
		MaxTokensPerExec: req.MaxTokensPerExec,
		AllowedModels:    req.AllowedModels,
		AllowedProviders: req.AllowedProviders,
		MaxCostPerExec:   req.MaxCostPerExec,
		MaxExecsPerDay:   req.MaxExecsPerDay,
		MaxExecsPerHour:  req.MaxExecsPerHour,
		Enabled:          true,
	}
	if req.Enabled != nil {
		policy.Enabled = *req.Enabled
	}
	return s.policyRepo.Upsert(ctx, policy)
}

func (s *governanceService) ListPolicies(ctx context.Context, orgID uuid.UUID) ([]model.AgentPolicy, error) {
	return s.policyRepo.List(ctx, orgID)
}

func (s *governanceService) DeletePolicy(ctx context.Context, policyID uuid.UUID) error {
	return s.policyRepo.Delete(ctx, policyID)
}

// ─── Helpers ────────────────────────────────────────────────────────────────

func contains(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
