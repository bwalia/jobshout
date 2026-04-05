package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

type LLMProviderRepository interface {
	Create(ctx context.Context, p *model.LLMProviderConfig) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.LLMProviderConfig, error)
	List(ctx context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateLLMProviderRequest) (*model.LLMProviderConfig, error)
	Delete(ctx context.Context, id uuid.UUID) error
	ClearDefault(ctx context.Context, orgID uuid.UUID) error
}

type llmProviderRepository struct {
	pool *pgxpool.Pool
}

func NewLLMProviderRepository(pool *pgxpool.Pool) LLMProviderRepository {
	return &llmProviderRepository{pool: pool}
}

func (r *llmProviderRepository) Create(ctx context.Context, p *model.LLMProviderConfig) error {
	configJSON, _ := json.Marshal(p.ConfigJSON)
	if configJSON == nil {
		configJSON = []byte("{}")
	}

	const sql = `
		INSERT INTO llm_provider_configs
		    (id, org_id, name, provider_type, base_url, api_key, default_model, is_default, is_active, config_json, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, sql,
		p.ID, p.OrgID, p.Name, p.ProviderType, p.BaseURL, p.APIKey,
		p.DefaultModel, p.IsDefault, p.IsActive, configJSON, p.CreatedBy,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
}

func (r *llmProviderRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.LLMProviderConfig, error) {
	const sql = `
		SELECT id, org_id, name, provider_type, base_url, api_key, default_model,
		       is_default, is_active, config_json, created_by, created_at, updated_at
		FROM llm_provider_configs WHERE id = $1`

	p := &model.LLMProviderConfig{}
	var configRaw []byte
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.BaseURL, &p.APIKey,
		&p.DefaultModel, &p.IsDefault, &p.IsActive, &configRaw, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("llm_provider_repo: get by id: %w", err)
	}
	_ = json.Unmarshal(configRaw, &p.ConfigJSON)
	return p, nil
}

func (r *llmProviderRepository) List(ctx context.Context, orgID uuid.UUID) ([]model.LLMProviderConfig, error) {
	const sql = `
		SELECT id, org_id, name, provider_type, base_url, api_key, default_model,
		       is_default, is_active, config_json, created_by, created_at, updated_at
		FROM llm_provider_configs WHERE org_id = $1 ORDER BY is_default DESC, name`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("llm_provider_repo: list: %w", err)
	}
	defer rows.Close()

	var providers []model.LLMProviderConfig
	for rows.Next() {
		var p model.LLMProviderConfig
		var configRaw []byte
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.BaseURL, &p.APIKey,
			&p.DefaultModel, &p.IsDefault, &p.IsActive, &configRaw, &p.CreatedBy,
			&p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("llm_provider_repo: scan: %w", err)
		}
		_ = json.Unmarshal(configRaw, &p.ConfigJSON)
		// Mask the API key in list responses
		if len(p.APIKey) > 8 {
			p.APIKey = p.APIKey[:4] + "****" + p.APIKey[len(p.APIKey)-4:]
		} else if p.APIKey != "" {
			p.APIKey = "****"
		}
		providers = append(providers, p)
	}
	return providers, rows.Err()
}

func (r *llmProviderRepository) Update(ctx context.Context, id uuid.UUID, req model.UpdateLLMProviderRequest) (*model.LLMProviderConfig, error) {
	// Build dynamic update
	const sql = `
		UPDATE llm_provider_configs SET
		    name = COALESCE($2, name),
		    base_url = COALESCE($3, base_url),
		    api_key = COALESCE($4, api_key),
		    default_model = COALESCE($5, default_model),
		    is_default = COALESCE($6, is_default),
		    is_active = COALESCE($7, is_active),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, org_id, name, provider_type, base_url, default_model,
		          is_default, is_active, created_by, created_at, updated_at`

	p := &model.LLMProviderConfig{}
	err := r.pool.QueryRow(ctx, sql,
		id, req.Name, req.BaseURL, req.APIKey, req.DefaultModel, req.IsDefault, req.IsActive,
	).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.ProviderType, &p.BaseURL,
		&p.DefaultModel, &p.IsDefault, &p.IsActive, &p.CreatedBy,
		&p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("llm_provider_repo: update: %w", err)
	}
	return p, nil
}

func (r *llmProviderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM llm_provider_configs WHERE id = $1", id)
	return err
}

func (r *llmProviderRepository) ClearDefault(ctx context.Context, orgID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE llm_provider_configs SET is_default = false WHERE org_id = $1 AND is_default = true",
		orgID)
	return err
}
