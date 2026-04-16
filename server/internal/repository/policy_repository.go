package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// PolicyRepository handles persistence for agent governance policies.
type PolicyRepository interface {
	Upsert(ctx context.Context, policy *model.AgentPolicy) (*model.AgentPolicy, error)
	GetForAgent(ctx context.Context, orgID, agentID uuid.UUID) (*model.AgentPolicy, error)
	GetOrgDefault(ctx context.Context, orgID uuid.UUID) (*model.AgentPolicy, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.AgentPolicy, error)
	Delete(ctx context.Context, id uuid.UUID) error
}

type policyRepository struct {
	pool *pgxpool.Pool
}

// NewPolicyRepository creates a PolicyRepository backed by pgxpool.
func NewPolicyRepository(pool *pgxpool.Pool) PolicyRepository {
	return &policyRepository{pool: pool}
}

const policyCols = `id, org_id, agent_id, max_tokens_per_exec, allowed_models, allowed_providers,
	max_cost_per_exec, max_execs_per_day, max_execs_per_hour, enabled, created_at, updated_at`

func scanPolicy(row pgx.Row) (*model.AgentPolicy, error) {
	p := &model.AgentPolicy{}
	if err := row.Scan(
		&p.ID, &p.OrgID, &p.AgentID, &p.MaxTokensPerExec, &p.AllowedModels, &p.AllowedProviders,
		&p.MaxCostPerExec, &p.MaxExecsPerDay, &p.MaxExecsPerHour, &p.Enabled, &p.CreatedAt, &p.UpdatedAt,
	); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *policyRepository) Upsert(ctx context.Context, policy *model.AgentPolicy) (*model.AgentPolicy, error) {
	const sql = `
		INSERT INTO agent_policies
		    (org_id, agent_id, max_tokens_per_exec, allowed_models, allowed_providers,
		     max_cost_per_exec, max_execs_per_day, max_execs_per_hour, enabled, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
		ON CONFLICT (org_id, COALESCE(agent_id, '00000000-0000-0000-0000-000000000000'::uuid))
		DO UPDATE SET
		    max_tokens_per_exec = EXCLUDED.max_tokens_per_exec,
		    allowed_models      = EXCLUDED.allowed_models,
		    allowed_providers   = EXCLUDED.allowed_providers,
		    max_cost_per_exec   = EXCLUDED.max_cost_per_exec,
		    max_execs_per_day   = EXCLUDED.max_execs_per_day,
		    max_execs_per_hour  = EXCLUDED.max_execs_per_hour,
		    enabled             = EXCLUDED.enabled,
		    updated_at          = NOW()
		RETURNING ` + policyCols

	row := r.pool.QueryRow(ctx, sql,
		policy.OrgID, policy.AgentID, policy.MaxTokensPerExec,
		policy.AllowedModels, policy.AllowedProviders,
		policy.MaxCostPerExec, policy.MaxExecsPerDay, policy.MaxExecsPerHour, policy.Enabled,
	)
	out, err := scanPolicy(row)
	if err != nil {
		return nil, fmt.Errorf("policy_repo: upsert: %w", err)
	}
	return out, nil
}

// GetForAgent returns the agent-specific policy. Returns nil, nil if none exists.
func (r *policyRepository) GetForAgent(ctx context.Context, orgID, agentID uuid.UUID) (*model.AgentPolicy, error) {
	sql := `SELECT ` + policyCols + `
		FROM agent_policies WHERE org_id = $1 AND agent_id = $2 AND enabled = true`

	row := r.pool.QueryRow(ctx, sql, orgID, agentID)
	out, err := scanPolicy(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("policy_repo: get for agent: %w", err)
	}
	return out, nil
}

// GetOrgDefault returns the org-wide default policy (agent_id IS NULL). Returns nil, nil if none exists.
func (r *policyRepository) GetOrgDefault(ctx context.Context, orgID uuid.UUID) (*model.AgentPolicy, error) {
	sql := `SELECT ` + policyCols + `
		FROM agent_policies WHERE org_id = $1 AND agent_id IS NULL AND enabled = true`

	row := r.pool.QueryRow(ctx, sql, orgID)
	out, err := scanPolicy(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("policy_repo: get org default: %w", err)
	}
	return out, nil
}

func (r *policyRepository) List(ctx context.Context, orgID uuid.UUID) ([]model.AgentPolicy, error) {
	sql := `SELECT ` + policyCols + `
		FROM agent_policies WHERE org_id = $1 ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("policy_repo: list: %w", err)
	}
	defer rows.Close()

	var policies []model.AgentPolicy
	for rows.Next() {
		var p model.AgentPolicy
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.AgentID, &p.MaxTokensPerExec, &p.AllowedModels, &p.AllowedProviders,
			&p.MaxCostPerExec, &p.MaxExecsPerDay, &p.MaxExecsPerHour, &p.Enabled, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("policy_repo: scan: %w", err)
		}
		policies = append(policies, p)
	}
	return policies, rows.Err()
}

func (r *policyRepository) Delete(ctx context.Context, id uuid.UUID) error {
	const sql = `DELETE FROM agent_policies WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("policy_repo: delete: %w", err)
	}
	return nil
}
