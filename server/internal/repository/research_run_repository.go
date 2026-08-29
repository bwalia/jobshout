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

// ResearchRunRepository persists Research Agent runs.
//
// Runs are written by every research entry point — Task Manager, chat, mail and
// the article pipeline — so the agent board and the run API have a single table
// to read.
type ResearchRunRepository interface {
	Create(ctx context.Context, r *model.ResearchRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ResearchRun, error)
	Update(ctx context.Context, r *model.ResearchRun) error
	// UpdatePhase writes only the live phase. It is called on every progress
	// callback, so it deliberately does not rewrite the brief or the status.
	UpdatePhase(ctx context.Context, id uuid.UUID, phase string) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.ResearchRun], error)
}

type researchRunRepository struct {
	pool *pgxpool.Pool
}

func NewResearchRunRepository(pool *pgxpool.Pool) ResearchRunRepository {
	return &researchRunRepository{pool: pool}
}

const researchRunColumns = `
	id, org_id, agent_id, task_id, requested_by, source,
	topic, context, urls, status, phase, brief, usable, error_message,
	started_at, completed_at, created_at, updated_at`

func (r *researchRunRepository) Create(ctx context.Context, run *model.ResearchRun) error {
	if run.ID == uuid.Nil {
		run.ID = uuid.New()
	}
	if run.Status == "" {
		run.Status = model.ResearchRunQueued
	}
	if run.Source == "" {
		run.Source = model.ResearchSourceAPI
	}
	if run.URLs == nil {
		run.URLs = []string{}
	}
	err := r.pool.QueryRow(ctx, `
		INSERT INTO research_runs (
			id, org_id, agent_id, task_id, requested_by, source,
			topic, context, urls, status, phase, brief, usable, error_message,
			started_at, completed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16)
		RETURNING created_at, updated_at`,
		run.ID, run.OrgID, run.AgentID, run.TaskID, run.RequestedBy, run.Source,
		run.Topic, run.Context, run.URLs, run.Status, run.Phase, run.Brief, run.Usable, run.ErrorMessage,
		run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
	if err != nil {
		return fmt.Errorf("research_run_repo: create: %w", err)
	}
	return nil
}

func (r *researchRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ResearchRun, error) {
	row := r.pool.QueryRow(ctx, `SELECT `+researchRunColumns+` FROM research_runs WHERE id = $1`, id)
	run, err := scanResearchRun(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("research_run_repo: get: %w", err)
	}
	return run, nil
}

func (r *researchRunRepository) Update(ctx context.Context, run *model.ResearchRun) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE research_runs SET
			status = $2, phase = $3, brief = $4, usable = $5, error_message = $6,
			started_at = $7, completed_at = $8, updated_at = NOW()
		WHERE id = $1`,
		run.ID, run.Status, run.Phase, run.Brief, run.Usable, run.ErrorMessage,
		run.StartedAt, run.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("research_run_repo: update: %w", err)
	}
	return nil
}

func (r *researchRunRepository) UpdatePhase(ctx context.Context, id uuid.UUID, phase string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE research_runs SET phase = $2, updated_at = NOW() WHERE id = $1`, id, phase)
	if err != nil {
		return fmt.Errorf("research_run_repo: update phase: %w", err)
	}
	return nil
}

func (r *researchRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, p model.PaginationParams) (*model.PaginatedResponse[model.ResearchRun], error) {
	p.Normalize()
	page, perPage := p.Page, p.PerPage

	var total int
	if err := r.pool.QueryRow(ctx,
		`SELECT COUNT(*) FROM research_runs WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("research_run_repo: count: %w", err)
	}

	rows, err := r.pool.Query(ctx, `SELECT `+researchRunColumns+`
		FROM research_runs WHERE org_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`,
		orgID, perPage, (page-1)*perPage)
	if err != nil {
		return nil, fmt.Errorf("research_run_repo: list: %w", err)
	}
	defer rows.Close()

	out := make([]model.ResearchRun, 0, perPage)
	for rows.Next() {
		run, err := scanResearchRun(rows)
		if err != nil {
			return nil, fmt.Errorf("research_run_repo: scan: %w", err)
		}
		out = append(out, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("research_run_repo: rows: %w", err)
	}
	pages := 0
	if perPage > 0 {
		pages = (total + perPage - 1) / perPage
	}
	return &model.PaginatedResponse[model.ResearchRun]{
		Data: out, Total: total, Page: page, PerPage: perPage, TotalPages: pages,
	}, nil
}

// scanner is satisfied by both pgx.Row and pgx.Rows, so one scan body serves
// the single-row and list paths.
type scanner interface {
	Scan(dest ...any) error
}

func scanResearchRun(s scanner) (*model.ResearchRun, error) {
	var run model.ResearchRun
	err := s.Scan(
		&run.ID, &run.OrgID, &run.AgentID, &run.TaskID, &run.RequestedBy, &run.Source,
		&run.Topic, &run.Context, &run.URLs, &run.Status, &run.Phase, &run.Brief,
		&run.Usable, &run.ErrorMessage, &run.StartedAt, &run.CompletedAt,
		&run.CreatedAt, &run.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &run, nil
}
