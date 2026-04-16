package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// AuditRepository handles audit log and login audit persistence.
type AuditRepository interface {
	RecordAction(ctx context.Context, log *model.AuditLog) error
	ListActions(ctx context.Context, orgID uuid.UUID, params model.AuditQueryParams) ([]model.AuditLog, error)
	RecordLogin(ctx context.Context, log *model.LoginAuditLog) error
	ListLogins(ctx context.Context, orgID uuid.UUID, limit int) ([]model.LoginAuditLog, error)
	RecordUsage(ctx context.Context, rec *model.UsageRecord) error
	RecordCost(ctx context.Context, rec *model.CostRecord) error
}

type auditRepository struct {
	pool *pgxpool.Pool
}

func NewAuditRepository(pool *pgxpool.Pool) AuditRepository {
	return &auditRepository{pool: pool}
}

func (r *auditRepository) RecordAction(ctx context.Context, log *model.AuditLog) error {
	oldJSON, _ := json.Marshal(log.OldValue)
	newJSON, _ := json.Marshal(log.NewValue)
	metaJSON, _ := json.Marshal(log.Metadata)

	const sql = `
		INSERT INTO audit_logs (org_id, user_id, action, resource, resource_id, cost_usd,
		    old_value, new_value, metadata, ip_address)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`

	_, err := r.pool.Exec(ctx, sql,
		log.OrgID, log.UserID, log.Action, log.Resource, log.ResourceID, log.CostUSD,
		oldJSON, newJSON, metaJSON, log.IPAddress,
	)
	if err != nil {
		return fmt.Errorf("audit_repo: record action: %w", err)
	}
	return nil
}

func (r *auditRepository) ListActions(ctx context.Context, orgID uuid.UUID, params model.AuditQueryParams) ([]model.AuditLog, error) {
	limit := params.Limit
	if limit <= 0 {
		limit = 100
	}

	const sql = `
		SELECT id, org_id, user_id, action, resource, resource_id, cost_usd,
		    old_value, new_value, metadata, ip_address, created_at
		FROM audit_logs WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, sql, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("audit_repo: list actions: %w", err)
	}
	defer rows.Close()

	var logs []model.AuditLog
	for rows.Next() {
		var l model.AuditLog
		var oldRaw, newRaw, metaRaw []byte
		if err := rows.Scan(
			&l.ID, &l.OrgID, &l.UserID, &l.Action, &l.Resource, &l.ResourceID, &l.CostUSD,
			&oldRaw, &newRaw, &metaRaw, &l.IPAddress, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("audit_repo: scan: %w", err)
		}
		_ = json.Unmarshal(oldRaw, &l.OldValue)
		_ = json.Unmarshal(newRaw, &l.NewValue)
		_ = json.Unmarshal(metaRaw, &l.Metadata)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *auditRepository) RecordLogin(ctx context.Context, log *model.LoginAuditLog) error {
	metaJSON, _ := json.Marshal(log.Metadata)

	const sql = `
		INSERT INTO login_audit_logs (user_id, org_id, email, provider, ip_address, user_agent, status, error_msg, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

	_, err := r.pool.Exec(ctx, sql,
		log.UserID, log.OrgID, log.Email, log.Provider,
		log.IPAddress, log.UserAgent, log.Status, log.ErrorMsg, metaJSON,
	)
	if err != nil {
		return fmt.Errorf("audit_repo: record login: %w", err)
	}
	return nil
}

func (r *auditRepository) ListLogins(ctx context.Context, orgID uuid.UUID, limit int) ([]model.LoginAuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	const sql = `
		SELECT id, user_id, org_id, email, provider, ip_address, user_agent, status, error_msg, metadata, created_at
		FROM login_audit_logs WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`

	rows, err := r.pool.Query(ctx, sql, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("audit_repo: list logins: %w", err)
	}
	defer rows.Close()

	var logs []model.LoginAuditLog
	for rows.Next() {
		var l model.LoginAuditLog
		var metaRaw []byte
		if err := rows.Scan(
			&l.ID, &l.UserID, &l.OrgID, &l.Email, &l.Provider,
			&l.IPAddress, &l.UserAgent, &l.Status, &l.ErrorMsg, &metaRaw, &l.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("audit_repo: scan login: %w", err)
		}
		_ = json.Unmarshal(metaRaw, &l.Metadata)
		logs = append(logs, l)
	}
	return logs, rows.Err()
}

func (r *auditRepository) RecordUsage(ctx context.Context, rec *model.UsageRecord) error {
	metaJSON, _ := json.Marshal(rec.Metadata)
	const sql = `
		INSERT INTO usage_records (org_id, agent_id, execution_id, task_id, user_id,
		    provider, model, tokens_in, tokens_out, latency_ms, cost_usd,
		    tool_calls, retries, is_error, metadata)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`

	_, err := r.pool.Exec(ctx, sql,
		rec.OrgID, rec.AgentID, rec.ExecutionID, rec.TaskID, rec.UserID,
		rec.Provider, rec.Model, rec.TokensIn, rec.TokensOut, rec.LatencyMs,
		rec.CostUSD, rec.ToolCalls, rec.Retries, rec.IsError, metaJSON,
	)
	if err != nil {
		return fmt.Errorf("audit_repo: record usage: %w", err)
	}
	return nil
}

func (r *auditRepository) RecordCost(ctx context.Context, rec *model.CostRecord) error {
	breakdownJSON, _ := json.Marshal(rec.Breakdown)
	const sql = `
		INSERT INTO cost_records (org_id, execution_id, task_id, agent_id, cost_type,
		    llm_cost_usd, tool_cost_usd, compute_cost_usd, total_cost_usd,
		    provider, model, breakdown)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`

	_, err := r.pool.Exec(ctx, sql,
		rec.OrgID, rec.ExecutionID, rec.TaskID, rec.AgentID, rec.CostType,
		rec.LLMCostUSD, rec.ToolCostUSD, rec.ComputeCostUSD, rec.TotalCostUSD,
		rec.Provider, rec.Model, breakdownJSON,
	)
	if err != nil {
		return fmt.Errorf("audit_repo: record cost: %w", err)
	}
	return nil
}
