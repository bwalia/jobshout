package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// BlogRepository persists blog_runs.
type BlogRepository interface {
	Create(ctx context.Context, run *model.BlogRun) error
	Update(ctx context.Context, run *model.BlogRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error)
}

type blogRepository struct {
	pool *pgxpool.Pool
}

// NewBlogRepository creates a BlogRepository backed by pgxpool.
func NewBlogRepository(pool *pgxpool.Pool) BlogRepository {
	return &blogRepository{pool: pool}
}

func (r *blogRepository) Create(ctx context.Context, run *model.BlogRun) error {
	topicsJSON, _ := json.Marshal(run.Topics)
	articlesJSON, _ := json.Marshal(run.Articles)

	const sql = `
		INSERT INTO blog_runs
		    (id, org_id, triggered_by, source, status, topics, model, articles, started_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, NOW())
		RETURNING created_at`

	return r.pool.QueryRow(ctx, sql,
		run.ID, run.OrgID, run.TriggeredBy, run.Source, run.Status,
		topicsJSON, run.Model, articlesJSON, run.StartedAt,
	).Scan(&run.CreatedAt)
}

func (r *blogRepository) Update(ctx context.Context, run *model.BlogRun) error {
	articlesJSON, _ := json.Marshal(run.Articles)

	const sql = `
		UPDATE blog_runs SET
		    status        = $2,
		    branch        = $3,
		    pr_number     = $4,
		    pr_url        = $5,
		    articles      = $6,
		    error_message = $7,
		    completed_at  = $8
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, sql,
		run.ID, run.Status, run.Branch, run.PRNumber, run.PRURL,
		articlesJSON, run.ErrorMessage, run.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("blog_repo: update: %w", err)
	}
	return nil
}

func (r *blogRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error) {
	const sql = `
		SELECT id, org_id, triggered_by, source, status, topics, model,
		       branch, pr_number, pr_url, articles, error_message,
		       started_at, completed_at, created_at
		FROM blog_runs WHERE id = $1`

	run := &model.BlogRun{}
	var topicsRaw, articlesRaw []byte
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&run.ID, &run.OrgID, &run.TriggeredBy, &run.Source, &run.Status,
		&topicsRaw, &run.Model,
		&run.Branch, &run.PRNumber, &run.PRURL,
		&articlesRaw, &run.ErrorMessage,
		&run.StartedAt, &run.CompletedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("blog_repo: get: %w", err)
	}
	_ = json.Unmarshal(topicsRaw, &run.Topics)
	_ = json.Unmarshal(articlesRaw, &run.Articles)
	return run, nil
}

func (r *blogRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM blog_runs WHERE org_id = $1", orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("blog_repo: count: %w", err)
	}

	const sql = `
		SELECT id, org_id, triggered_by, source, status, topics, model,
		       branch, pr_number, pr_url, articles, error_message,
		       started_at, completed_at, created_at
		FROM blog_runs WHERE org_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("blog_repo: list: %w", err)
	}
	defer rows.Close()

	var runs []model.BlogRun
	for rows.Next() {
		var run model.BlogRun
		var topicsRaw, articlesRaw []byte
		if err := rows.Scan(
			&run.ID, &run.OrgID, &run.TriggeredBy, &run.Source, &run.Status,
			&topicsRaw, &run.Model,
			&run.Branch, &run.PRNumber, &run.PRURL,
			&articlesRaw, &run.ErrorMessage,
			&run.StartedAt, &run.CompletedAt, &run.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("blog_repo: scan: %w", err)
		}
		_ = json.Unmarshal(topicsRaw, &run.Topics)
		_ = json.Unmarshal(articlesRaw, &run.Articles)
		runs = append(runs, run)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.BlogRun]{
		Data: runs, Total: total, Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, rows.Err()
}
