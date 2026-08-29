package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// AgentRunRepository persists agent runs — the record of "agent X ran with
// inputs Y", whichever surface asked for it.
type AgentRunRepository interface {
	Create(ctx context.Context, r *model.AgentRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.AgentRun, error)
	Update(ctx context.Context, r *model.AgentRun) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.AgentRun], error)
}

type agentRunRepository struct {
	pool *pgxpool.Pool
}

func NewAgentRunRepository(pool *pgxpool.Pool) AgentRunRepository {
	return &agentRunRepository{pool: pool}
}

const agentRunColumns = `
	id, org_id, agent_id, task_id, requested_by, builtin, source, inputs,
	status, external_run_id, external_kind, error_message,
	started_at, completed_at, created_at, updated_at`

func (r *agentRunRepository) Create(ctx context.Context, run *model.AgentRun) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.Status == "" {
		run.Status = model.AgentRunQueued
	}
	if run.Source == "" {
		run.Source = model.AgentRunSourceAPI
	}
	if len(run.Inputs) == 0 {
		run.Inputs = []byte(`{}`)
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO agent_runs (
			id, org_id, agent_id, task_id, requested_by, builtin, source, inputs,
			status, external_run_id, external_kind, error_message, started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
		RETURNING created_at, updated_at`,
		run.ID, run.OrgID, run.AgentID, run.TaskID, run.RequestedBy, run.Builtin, run.Source, run.Inputs,
		run.Status, run.ExternalRunID, run.ExternalKind, run.ErrorMessage, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return fmt.Errorf("agent_run_repo: create: %w", err)
	}
	return nil
}

func (r *agentRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.AgentRun, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+agentRunColumns+` FROM agent_runs WHERE id = $1`, id)
	run, err := scanAgentRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("agent_run_repo: get: %w", err)
	}
	return run, nil
}

func (r *agentRunRepository) Update(ctx context.Context, run *model.AgentRun) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE agent_runs SET
			status = $2, external_run_id = $3, external_kind = $4, error_message = $5,
			started_at = $6, completed_at = $7, updated_at = NOW()
		WHERE id = $1`,
		run.ID, run.Status, run.ExternalRunID, run.ExternalKind, run.ErrorMessage,
		run.StartedAt, run.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("agent_run_repo: update: %w", err)
	}
	return nil
}

func (r *agentRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, p model.PaginationParams) (*model.PaginatedResponse[model.AgentRun], error) {
	p.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM agent_runs WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("agent_run_repo: count: %w", err)
	}

	rows, err := r.pool.Query(ctx, `SELECT `+agentRunColumns+`
		FROM agent_runs WHERE org_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, p.PerPage, p.Offset())
	if err != nil {
		return nil, fmt.Errorf("agent_run_repo: list: %w", err)
	}
	defer rows.Close()

	out := make([]model.AgentRun, 0, p.PerPage)
	for rows.Next() {
		run, err := scanAgentRun(rows)
		if err != nil {
			return nil, fmt.Errorf("agent_run_repo: scan: %w", err)
		}
		out = append(out, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("agent_run_repo: rows: %w", err)
	}
	pages := 0
	if p.PerPage > 0 {
		pages = (total + p.PerPage - 1) / p.PerPage
	}
	return &model.PaginatedResponse[model.AgentRun]{
		Data: out, Total: total, Page: p.Page, PerPage: p.PerPage, TotalPages: pages,
	}, nil
}

func scanAgentRun(s scanner) (*model.AgentRun, error) {
	var run model.AgentRun
	err := s.Scan(
		&run.ID, &run.OrgID, &run.AgentID, &run.TaskID, &run.RequestedBy,
		&run.Builtin, &run.Source, &run.Inputs, &run.Status,
		&run.ExternalRunID, &run.ExternalKind, &run.ErrorMessage,
		&run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &run, nil
}
