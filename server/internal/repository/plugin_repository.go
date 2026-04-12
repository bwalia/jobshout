package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// PluginRepository handles persistence for plugins and plugin executions.
type PluginRepository interface {
	Create(ctx context.Context, p *model.Plugin) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Plugin, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Plugin], error)
	Update(ctx context.Context, p *model.Plugin) error
	Delete(ctx context.Context, id uuid.UUID) error
	CreateExecution(ctx context.Context, pe *model.PluginExecution) error
	UpdateExecution(ctx context.Context, pe *model.PluginExecution) error
	ListExecutions(ctx context.Context, pluginID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.PluginExecution], error)
}

type pluginRepository struct {
	pool *pgxpool.Pool
}

func NewPluginRepository(pool *pgxpool.Pool) PluginRepository {
	return &pluginRepository{pool: pool}
}

func (r *pluginRepository) Create(ctx context.Context, p *model.Plugin) error {
	wfDef, _ := json.Marshal(p.WorkflowDef)
	perms, _ := json.Marshal(p.Permissions)
	cfg, _ := json.Marshal(p.Config)

	if p.Version == "" {
		p.Version = "1.0.0"
	}
	if p.PluginType == "" {
		p.PluginType = "langgraph"
	}

	const sql = `
		INSERT INTO plugins (id, org_id, name, version, description, status, plugin_type,
			workflow_def, permissions, config, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, sql,
		p.ID, p.OrgID, p.Name, p.Version, p.Description, model.PluginStatusActive,
		p.PluginType, wfDef, perms, cfg, p.CreatedBy,
	).Scan(&p.CreatedAt, &p.UpdatedAt)
}

func (r *pluginRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Plugin, error) {
	const sql = `
		SELECT id, org_id, name, version, description, status, plugin_type,
			workflow_def, permissions, config, created_by, created_at, updated_at
		FROM plugins WHERE id = $1`

	p := &model.Plugin{}
	var wfDef, perms, cfg []byte
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Version, &p.Description, &p.Status, &p.PluginType,
		&wfDef, &perms, &cfg, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin_repo: find by id: %w", err)
	}
	_ = json.Unmarshal(wfDef, &p.WorkflowDef)
	_ = json.Unmarshal(perms, &p.Permissions)
	_ = json.Unmarshal(cfg, &p.Config)
	return p, nil
}

func (r *pluginRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Plugin], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM plugins WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("plugin_repo: count: %w", err)
	}

	const sql = `
		SELECT id, org_id, name, version, description, status, plugin_type,
			workflow_def, permissions, config, created_by, created_at, updated_at
		FROM plugins WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("plugin_repo: list: %w", err)
	}
	defer rows.Close()

	plugins := make([]model.Plugin, 0)
	for rows.Next() {
		var p model.Plugin
		var wfDef, perms, cfg []byte
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Version, &p.Description, &p.Status, &p.PluginType,
			&wfDef, &perms, &cfg, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("plugin_repo: scan: %w", err)
		}
		_ = json.Unmarshal(wfDef, &p.WorkflowDef)
		_ = json.Unmarshal(perms, &p.Permissions)
		_ = json.Unmarshal(cfg, &p.Config)
		plugins = append(plugins, p)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.Plugin]{
		Data: plugins, Total: total,
		Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, nil
}

func (r *pluginRepository) Update(ctx context.Context, p *model.Plugin) error {
	wfDef, _ := json.Marshal(p.WorkflowDef)
	perms, _ := json.Marshal(p.Permissions)
	cfg, _ := json.Marshal(p.Config)

	const sql = `
		UPDATE plugins SET name=$1, version=$2, description=$3, status=$4, plugin_type=$5,
			workflow_def=$6, permissions=$7, config=$8, updated_at=NOW()
		WHERE id = $9 RETURNING updated_at`

	return r.pool.QueryRow(ctx, sql,
		p.Name, p.Version, p.Description, p.Status, p.PluginType,
		wfDef, perms, cfg, p.ID,
	).Scan(&p.UpdatedAt)
}

func (r *pluginRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM plugins WHERE id = $1`, id)
	return err
}

func (r *pluginRepository) CreateExecution(ctx context.Context, pe *model.PluginExecution) error {
	inputJSON, _ := json.Marshal(pe.Input)

	const sql = `
		INSERT INTO plugin_executions (id, plugin_id, execution_id, org_id, input, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())`

	_, err := r.pool.Exec(ctx, sql,
		pe.ID, pe.PluginID, pe.ExecutionID, pe.OrgID, inputJSON, model.ExecutionStatusPending,
	)
	return err
}

func (r *pluginRepository) UpdateExecution(ctx context.Context, pe *model.PluginExecution) error {
	const sql = `
		UPDATE plugin_executions SET output=$1, status=$2, error_message=$3,
			started_at=$4, completed_at=$5 WHERE id = $6`

	_, err := r.pool.Exec(ctx, sql,
		pe.Output, pe.Status, pe.ErrorMessage, pe.StartedAt, pe.CompletedAt, pe.ID,
	)
	return err
}

func (r *pluginRepository) ListExecutions(ctx context.Context, pluginID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.PluginExecution], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM plugin_executions WHERE plugin_id = $1`, pluginID).Scan(&total); err != nil {
		return nil, fmt.Errorf("plugin_repo: count executions: %w", err)
	}

	const sql = `
		SELECT id, plugin_id, execution_id, org_id, input, output, status, error_message,
			started_at, completed_at, created_at
		FROM plugin_executions WHERE plugin_id = $1 ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, pluginID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("plugin_repo: list executions: %w", err)
	}
	defer rows.Close()

	execs := make([]model.PluginExecution, 0)
	for rows.Next() {
		var pe model.PluginExecution
		var inputRaw []byte
		if err := rows.Scan(
			&pe.ID, &pe.PluginID, &pe.ExecutionID, &pe.OrgID,
			&inputRaw, &pe.Output, &pe.Status, &pe.ErrorMessage,
			&pe.StartedAt, &pe.CompletedAt, &pe.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("plugin_repo: scan execution: %w", err)
		}
		_ = json.Unmarshal(inputRaw, &pe.Input)
		execs = append(execs, pe)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.PluginExecution]{
		Data: execs, Total: total,
		Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, nil
}

// Compile-time interface check.
var _ PluginRepository = (*pluginRepository)(nil)

// Needed for the time import.
var _ = time.Now
