package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// AgentListFilter holds optional filter criteria for listing agents.
type AgentListFilter struct {
	Search string
	Status string
}

type AgentRepository interface {
	Create(ctx context.Context, agent *model.Agent) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Agent, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams, filter AgentListFilter) (*model.PaginatedResponse[model.Agent], error)
	Update(ctx context.Context, agent *model.Agent) error
	Delete(ctx context.Context, id uuid.UUID) error
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

type agentRepository struct {
	pool *pgxpool.Pool
}

func NewAgentRepository(pool *pgxpool.Pool) AgentRepository {
	return &agentRepository{pool: pool}
}

func (r *agentRepository) Create(ctx context.Context, agent *model.Agent) error {
	if agent.EngineType == "" {
		agent.EngineType = model.EngineGoNative
	}
	engineConfigJSON, err := json.Marshal(agent.EngineConfig)
	if err != nil {
		engineConfigJSON = []byte("{}")
	}

	query := `
		INSERT INTO agents (id, org_id, name, role, description, avatar_url, status,
			model_provider, model_name, system_prompt, performance_score, manager_id, created_by,
			engine_type, engine_config, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		agent.ID, agent.OrgID, agent.Name, agent.Role, agent.Description, agent.AvatarURL,
		agent.Status, agent.ModelProvider, agent.ModelName, agent.SystemPrompt,
		agent.PerformanceScore, agent.ManagerID, agent.CreatedBy,
		agent.EngineType, engineConfigJSON,
	).Scan(&agent.CreatedAt, &agent.UpdatedAt)
}

func (r *agentRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Agent, error) {
	query := `
		SELECT id, org_id, name, role, description, avatar_url, status,
			model_provider, model_name, system_prompt, performance_score,
			manager_id, created_by, engine_type, engine_config, created_at, updated_at
		FROM agents WHERE id = $1`

	a := &model.Agent{}
	var engineConfigRaw []byte
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&a.ID, &a.OrgID, &a.Name, &a.Role, &a.Description, &a.AvatarURL,
		&a.Status, &a.ModelProvider, &a.ModelName, &a.SystemPrompt,
		&a.PerformanceScore, &a.ManagerID, &a.CreatedBy,
		&a.EngineType, &engineConfigRaw, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding agent by id: %w", err)
	}
	if len(engineConfigRaw) > 0 {
		_ = json.Unmarshal(engineConfigRaw, &a.EngineConfig)
	}
	if a.EngineConfig == nil {
		a.EngineConfig = map[string]any{}
	}
	return a, nil
}

func (r *agentRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams, filter AgentListFilter) (*model.PaginatedResponse[model.Agent], error) {
	params.Normalize()

	// Build dynamic WHERE clause
	conditions := []string{"org_id = $1"}
	args := []any{orgID}
	argIdx := 2

	if filter.Status != "" {
		conditions = append(conditions, fmt.Sprintf("status = $%d", argIdx))
		args = append(args, filter.Status)
		argIdx++
	}
	if filter.Search != "" {
		conditions = append(conditions, fmt.Sprintf("(LOWER(name) LIKE $%d OR LOWER(role) LIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+strings.ToLower(filter.Search)+"%")
		argIdx++
	}

	whereClause := strings.Join(conditions, " AND ")

	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM agents WHERE %s`, whereClause)
	if err := r.pool.QueryRow(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting agents: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, org_id, name, role, description, avatar_url, status,
			model_provider, model_name, system_prompt, performance_score,
			manager_id, created_by, engine_type, engine_config, created_at, updated_at
		FROM agents WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`, whereClause, argIdx, argIdx+1)

	args = append(args, params.PerPage, params.Offset())

	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing agents: %w", err)
	}
	defer rows.Close()

	agents := make([]model.Agent, 0)
	for rows.Next() {
		var a model.Agent
		var engineConfigRaw []byte
		if err := rows.Scan(
			&a.ID, &a.OrgID, &a.Name, &a.Role, &a.Description, &a.AvatarURL,
			&a.Status, &a.ModelProvider, &a.ModelName, &a.SystemPrompt,
			&a.PerformanceScore, &a.ManagerID, &a.CreatedBy,
			&a.EngineType, &engineConfigRaw, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning agent row: %w", err)
		}
		if len(engineConfigRaw) > 0 {
			_ = json.Unmarshal(engineConfigRaw, &a.EngineConfig)
		}
		if a.EngineConfig == nil {
			a.EngineConfig = map[string]any{}
		}
		agents = append(agents, a)
	}

	totalPages := total / params.PerPage
	if total%params.PerPage != 0 {
		totalPages++
	}

	return &model.PaginatedResponse[model.Agent]{
		Data:       agents,
		Total:      total,
		Page:       params.Page,
		PerPage:    params.PerPage,
		TotalPages: totalPages,
	}, nil
}

func (r *agentRepository) Update(ctx context.Context, agent *model.Agent) error {
	engineConfigJSON, err := json.Marshal(agent.EngineConfig)
	if err != nil {
		engineConfigJSON = []byte("{}")
	}

	query := `
		UPDATE agents SET name = $1, role = $2, description = $3, avatar_url = $4,
			model_provider = $5, model_name = $6, system_prompt = $7, manager_id = $8,
			engine_type = $9, engine_config = $10, updated_at = NOW()
		WHERE id = $11
		RETURNING updated_at`

	return r.pool.QueryRow(ctx, query,
		agent.Name, agent.Role, agent.Description, agent.AvatarURL,
		agent.ModelProvider, agent.ModelName, agent.SystemPrompt, agent.ManagerID,
		agent.EngineType, engineConfigJSON, agent.ID,
	).Scan(&agent.UpdatedAt)
}

func (r *agentRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM agents WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("deleting agent: %w", err)
	}
	return nil
}

func (r *agentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE agents SET status = $1, updated_at = NOW() WHERE id = $2`, status, id)
	if err != nil {
		return fmt.Errorf("updating agent status: %w", err)
	}
	return nil
}
