package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// BudgetRepository handles persistence for organization budgets and alerts.
type BudgetRepository interface {
	Upsert(ctx context.Context, budget *model.OrgBudget) (*model.OrgBudget, error)
	GetByOrgAndPeriod(ctx context.Context, orgID uuid.UUID, period string) (*model.OrgBudget, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.OrgBudget, error)
	Delete(ctx context.Context, id uuid.UUID) error
	RecordAlert(ctx context.Context, alert *model.BudgetAlert) error
	ListAlerts(ctx context.Context, orgID uuid.UUID, limit int) ([]model.BudgetAlert, error)
}

type budgetRepository struct {
	pool *pgxpool.Pool
}

// NewBudgetRepository creates a BudgetRepository backed by pgxpool.
func NewBudgetRepository(pool *pgxpool.Pool) BudgetRepository {
	return &budgetRepository{pool: pool}
}

func (r *budgetRepository) Upsert(ctx context.Context, budget *model.OrgBudget) (*model.OrgBudget, error) {
	const sql = `
		INSERT INTO org_budgets (org_id, period, soft_limit_usd, hard_limit_usd, alert_threshold, enabled, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
		ON CONFLICT (org_id, period)
		DO UPDATE SET
		    soft_limit_usd  = EXCLUDED.soft_limit_usd,
		    hard_limit_usd  = EXCLUDED.hard_limit_usd,
		    alert_threshold = EXCLUDED.alert_threshold,
		    enabled         = EXCLUDED.enabled,
		    updated_at      = NOW()
		RETURNING id, org_id, period, soft_limit_usd, hard_limit_usd, alert_threshold, enabled, created_at, updated_at`

	out := &model.OrgBudget{}
	if err := r.pool.QueryRow(ctx, sql,
		budget.OrgID, budget.Period, budget.SoftLimitUSD, budget.HardLimitUSD,
		budget.AlertThreshold, budget.Enabled,
	).Scan(
		&out.ID, &out.OrgID, &out.Period, &out.SoftLimitUSD, &out.HardLimitUSD,
		&out.AlertThreshold, &out.Enabled, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("budget_repo: upsert: %w", err)
	}
	return out, nil
}

func (r *budgetRepository) GetByOrgAndPeriod(ctx context.Context, orgID uuid.UUID, period string) (*model.OrgBudget, error) {
	const sql = `
		SELECT id, org_id, period, soft_limit_usd, hard_limit_usd, alert_threshold, enabled, created_at, updated_at
		FROM org_budgets WHERE org_id = $1 AND period = $2 AND enabled = true`

	out := &model.OrgBudget{}
	if err := r.pool.QueryRow(ctx, sql, orgID, period).Scan(
		&out.ID, &out.OrgID, &out.Period, &out.SoftLimitUSD, &out.HardLimitUSD,
		&out.AlertThreshold, &out.Enabled, &out.CreatedAt, &out.UpdatedAt,
	); err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("budget_repo: get by org and period: %w", err)
	}
	return out, nil
}

func (r *budgetRepository) List(ctx context.Context, orgID uuid.UUID) ([]model.OrgBudget, error) {
	const sql = `
		SELECT id, org_id, period, soft_limit_usd, hard_limit_usd, alert_threshold, enabled, created_at, updated_at
		FROM org_budgets WHERE org_id = $1 ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("budget_repo: list: %w", err)
	}
	defer rows.Close()

	var budgets []model.OrgBudget
	for rows.Next() {
		var b model.OrgBudget
		if err := rows.Scan(
			&b.ID, &b.OrgID, &b.Period, &b.SoftLimitUSD, &b.HardLimitUSD,
			&b.AlertThreshold, &b.Enabled, &b.CreatedAt, &b.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("budget_repo: scan: %w", err)
		}
		budgets = append(budgets, b)
	}
	return budgets, rows.Err()
}

func (r *budgetRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const sql = `DELETE FROM org_budgets WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("budget_repo: delete: %w", err)
	}
	return nil
}

func (r *budgetRepository) RecordAlert(ctx context.Context, alert *model.BudgetAlert) error {
	const sql = `
		INSERT INTO budget_alerts (org_id, budget_id, alert_type, spend_usd, limit_usd)
		VALUES ($1, $2, $3, $4, $5)`

	_, err := r.pool.Exec(ctx, sql,
		alert.OrgID, alert.BudgetID, alert.AlertType, alert.SpendUSD, alert.LimitUSD,
	)
	if err != nil {
		return fmt.Errorf("budget_repo: record alert: %w", err)
	}
	return nil
}

func (r *budgetRepository) ListAlerts(ctx context.Context, orgID uuid.UUID, limit int) ([]model.BudgetAlert, error) {
	if limit <= 0 {
		limit = 50
	}
	const sql = `
		SELECT id, org_id, budget_id, alert_type, spend_usd, limit_usd, triggered_at
		FROM budget_alerts WHERE org_id = $1 ORDER BY triggered_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, sql, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("budget_repo: list alerts: %w", err)
	}
	defer rows.Close()

	var alerts []model.BudgetAlert
	for rows.Next() {
		var a model.BudgetAlert
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.BudgetID, &a.AlertType, &a.SpendUSD, &a.LimitUSD, &a.TriggeredAt,
		); err != nil {
			return nil, fmt.Errorf("budget_repo: scan alert: %w", err)
		}
		alerts = append(alerts, a)
	}
	return alerts, rows.Err()
}
