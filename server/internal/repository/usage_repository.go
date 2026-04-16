package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// UsageRepository handles persistence and querying of usage rollups for analytics.
type UsageRepository interface {
	UpsertRollup(ctx context.Context, rollup *model.UsageRollup) error
	QueryRollups(ctx context.Context, params model.UsageQueryParams) ([]model.UsageRollup, error)
	OrgSpendSummary(ctx context.Context, orgID uuid.UUID, from, to time.Time) (*model.OrgUsageSummary, error)
	AgentAnalytics(ctx context.Context, agentID uuid.UUID, from, to time.Time) (*model.AgentAnalytics, error)
	TopAgentsBySpend(ctx context.Context, orgID uuid.UUID, limit int, from, to time.Time) ([]model.AgentAnalytics, error)
	CurrentPeriodSpend(ctx context.Context, orgID uuid.UUID, period string) (float64, error)
	RecentExecCount(ctx context.Context, agentID uuid.UUID, window time.Duration) (int, error)
}

type usageRepository struct {
	pool *pgxpool.Pool
}

// NewUsageRepository creates a UsageRepository backed by pgxpool.
func NewUsageRepository(pool *pgxpool.Pool) UsageRepository {
	return &usageRepository{pool: pool}
}

func (r *usageRepository) UpsertRollup(ctx context.Context, rollup *model.UsageRollup) error {
	const sql = `
		INSERT INTO usage_rollups
		    (org_id, agent_id, model_provider, model_name, period_type, period_start,
		     exec_count, input_tokens, output_tokens, total_tokens, cost_usd,
		     avg_latency_ms, error_count, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW())
		ON CONFLICT (org_id, agent_id, model_provider, model_name, period_type, period_start)
		DO UPDATE SET
		    exec_count     = usage_rollups.exec_count + EXCLUDED.exec_count,
		    input_tokens   = usage_rollups.input_tokens + EXCLUDED.input_tokens,
		    output_tokens  = usage_rollups.output_tokens + EXCLUDED.output_tokens,
		    total_tokens   = usage_rollups.total_tokens + EXCLUDED.total_tokens,
		    cost_usd       = usage_rollups.cost_usd + EXCLUDED.cost_usd,
		    avg_latency_ms = (usage_rollups.avg_latency_ms * usage_rollups.exec_count + EXCLUDED.avg_latency_ms)
		                     / (usage_rollups.exec_count + EXCLUDED.exec_count),
		    error_count    = usage_rollups.error_count + EXCLUDED.error_count,
		    updated_at     = NOW()`

	_, err := r.pool.Exec(ctx, sql,
		rollup.OrgID, rollup.AgentID, rollup.ModelProvider, rollup.ModelName,
		rollup.PeriodType, rollup.PeriodStart,
		rollup.ExecCount, rollup.InputTokens, rollup.OutputTokens, rollup.TotalTokens,
		rollup.CostUSD, rollup.AvgLatencyMs, rollup.ErrorCount,
	)
	if err != nil {
		return fmt.Errorf("usage_repo: upsert rollup: %w", err)
	}
	return nil
}

func (r *usageRepository) QueryRollups(ctx context.Context, params model.UsageQueryParams) ([]model.UsageRollup, error) {
	var (
		clauses []string
		args    []any
		idx     = 1
	)

	addClause := func(clause string, val any) {
		clauses = append(clauses, fmt.Sprintf(clause, idx))
		args = append(args, val)
		idx++
	}

	if params.AgentID != nil {
		addClause("agent_id = $%d", *params.AgentID)
	}
	if params.Provider != "" {
		addClause("model_provider = $%d", params.Provider)
	}
	if params.Model != "" {
		addClause("model_name = $%d", params.Model)
	}
	if !params.From.IsZero() {
		addClause("period_start >= $%d", params.From)
	}
	if !params.To.IsZero() {
		addClause("period_start <= $%d", params.To)
	}
	granularity := "daily"
	if params.Granularity == "hourly" {
		granularity = "hourly"
	}
	addClause("period_type = $%d", granularity)

	where := ""
	if len(clauses) > 0 {
		where = "WHERE " + strings.Join(clauses, " AND ")
	}

	sql := fmt.Sprintf(`
		SELECT id, org_id, agent_id, model_provider, model_name, period_type, period_start,
		       exec_count, input_tokens, output_tokens, total_tokens, cost_usd,
		       avg_latency_ms, error_count, created_at, updated_at
		FROM usage_rollups %s ORDER BY period_start DESC LIMIT 1000`, where)

	rows, err := r.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("usage_repo: query rollups: %w", err)
	}
	defer rows.Close()

	var rollups []model.UsageRollup
	for rows.Next() {
		var ru model.UsageRollup
		if err := rows.Scan(
			&ru.ID, &ru.OrgID, &ru.AgentID, &ru.ModelProvider, &ru.ModelName,
			&ru.PeriodType, &ru.PeriodStart,
			&ru.ExecCount, &ru.InputTokens, &ru.OutputTokens, &ru.TotalTokens,
			&ru.CostUSD, &ru.AvgLatencyMs, &ru.ErrorCount, &ru.CreatedAt, &ru.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("usage_repo: scan rollup: %w", err)
		}
		rollups = append(rollups, ru)
	}
	return rollups, rows.Err()
}

func (r *usageRepository) OrgSpendSummary(ctx context.Context, orgID uuid.UUID, from, to time.Time) (*model.OrgUsageSummary, error) {
	const sql = `
		SELECT COALESCE(SUM(cost_usd), 0),
		       COALESCE(SUM(exec_count), 0),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0),
		       COALESCE(SUM(error_count), 0),
		       COALESCE(AVG(avg_latency_ms), 0)::int
		FROM usage_rollups
		WHERE org_id = $1 AND period_type = 'daily'
		  AND period_start >= $2 AND period_start <= $3`

	summary := &model.OrgUsageSummary{OrgID: orgID}
	if err := r.pool.QueryRow(ctx, sql, orgID, from, to).Scan(
		&summary.TotalCostUSD, &summary.TotalExecs,
		&summary.TotalInputTok, &summary.TotalOutputTok,
		&summary.TotalErrors, &summary.AvgLatencyMs,
	); err != nil {
		return nil, fmt.Errorf("usage_repo: org spend summary: %w", err)
	}

	// Provider breakdown.
	const breakdownSQL = `
		SELECT model_provider,
		       COALESCE(SUM(cost_usd), 0),
		       COALESCE(SUM(exec_count), 0),
		       COALESCE(SUM(input_tokens), 0),
		       COALESCE(SUM(output_tokens), 0)
		FROM usage_rollups
		WHERE org_id = $1 AND period_type = 'daily'
		  AND period_start >= $2 AND period_start <= $3
		GROUP BY model_provider ORDER BY SUM(cost_usd) DESC`

	rows, err := r.pool.Query(ctx, breakdownSQL, orgID, from, to)
	if err != nil {
		return nil, fmt.Errorf("usage_repo: provider breakdown: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var pb model.ProviderBreakdown
		if err := rows.Scan(&pb.Provider, &pb.CostUSD, &pb.ExecCount, &pb.InputTokens, &pb.OutputTokens); err != nil {
			return nil, fmt.Errorf("usage_repo: scan provider breakdown: %w", err)
		}
		summary.ByProvider = append(summary.ByProvider, pb)
	}
	return summary, rows.Err()
}

func (r *usageRepository) AgentAnalytics(ctx context.Context, agentID uuid.UUID, from, to time.Time) (*model.AgentAnalytics, error) {
	const sql = `
		SELECT COALESCE(SUM(exec_count), 0),
		       COALESCE(AVG(avg_latency_ms), 0)::int,
		       COALESCE(SUM(cost_usd), 0),
		       COALESCE(SUM(total_tokens), 0),
		       COALESCE(SUM(error_count), 0)
		FROM usage_rollups
		WHERE agent_id = $1 AND period_type = 'daily'
		  AND period_start >= $2 AND period_start <= $3`

	a := &model.AgentAnalytics{AgentID: agentID}
	if err := r.pool.QueryRow(ctx, sql, agentID, from, to).Scan(
		&a.TotalRuns, &a.AvgLatencyMs, &a.TotalCostUSD, &a.TotalTokens, &a.ErrorCount,
	); err != nil {
		return nil, fmt.Errorf("usage_repo: agent analytics: %w", err)
	}
	if a.TotalRuns > 0 {
		a.SuccessRate = float64(a.TotalRuns-a.ErrorCount) / float64(a.TotalRuns)
	}
	return a, nil
}

func (r *usageRepository) TopAgentsBySpend(ctx context.Context, orgID uuid.UUID, limit int, from, to time.Time) ([]model.AgentAnalytics, error) {
	if limit <= 0 {
		limit = 10
	}
	const sql = `
		SELECT ur.agent_id,
		       COALESCE(a.name, ''),
		       COALESCE(SUM(ur.exec_count), 0),
		       COALESCE(AVG(ur.avg_latency_ms), 0)::int,
		       COALESCE(SUM(ur.cost_usd), 0),
		       COALESCE(SUM(ur.total_tokens), 0),
		       COALESCE(SUM(ur.error_count), 0)
		FROM usage_rollups ur
		LEFT JOIN agents a ON a.id = ur.agent_id
		WHERE ur.org_id = $1 AND ur.period_type = 'daily'
		  AND ur.period_start >= $2 AND ur.period_start <= $3
		  AND ur.agent_id IS NOT NULL
		GROUP BY ur.agent_id, a.name
		ORDER BY SUM(ur.cost_usd) DESC
		LIMIT $4`

	rows, err := r.pool.Query(ctx, sql, orgID, from, to, limit)
	if err != nil {
		return nil, fmt.Errorf("usage_repo: top agents: %w", err)
	}
	defer rows.Close()

	var agents []model.AgentAnalytics
	for rows.Next() {
		var ag model.AgentAnalytics
		if err := rows.Scan(
			&ag.AgentID, &ag.AgentName, &ag.TotalRuns, &ag.AvgLatencyMs,
			&ag.TotalCostUSD, &ag.TotalTokens, &ag.ErrorCount,
		); err != nil {
			return nil, fmt.Errorf("usage_repo: scan top agent: %w", err)
		}
		if ag.TotalRuns > 0 {
			ag.SuccessRate = float64(ag.TotalRuns-ag.ErrorCount) / float64(ag.TotalRuns)
		}
		agents = append(agents, ag)
	}
	return agents, rows.Err()
}

func (r *usageRepository) CurrentPeriodSpend(ctx context.Context, orgID uuid.UUID, period string) (float64, error) {
	start := periodStart(period)
	const sql = `
		SELECT COALESCE(SUM(cost_usd), 0)
		FROM usage_rollups
		WHERE org_id = $1 AND period_type = 'daily' AND period_start >= $2`

	var spend float64
	if err := r.pool.QueryRow(ctx, sql, orgID, start).Scan(&spend); err != nil {
		return 0, fmt.Errorf("usage_repo: current period spend: %w", err)
	}
	return spend, nil
}

func (r *usageRepository) RecentExecCount(ctx context.Context, agentID uuid.UUID, window time.Duration) (int, error) {
	since := time.Now().Add(-window)
	const sql = `
		SELECT COUNT(*) FROM agent_executions
		WHERE agent_id = $1 AND created_at >= $2`

	var count int
	if err := r.pool.QueryRow(ctx, sql, agentID, since).Scan(&count); err != nil {
		return 0, fmt.Errorf("usage_repo: recent exec count: %w", err)
	}
	return count, nil
}

// periodStart returns the start of the current billing period.
func periodStart(period string) time.Time {
	now := time.Now().UTC()
	switch period {
	case model.BudgetPeriodDaily:
		return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	case model.BudgetPeriodYearly:
		return time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	default: // monthly
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	}
}
