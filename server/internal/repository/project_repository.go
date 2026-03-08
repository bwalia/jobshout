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

type ProjectRepository interface {
	Create(ctx context.Context, project *model.Project) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Project, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Project], error)
	Update(ctx context.Context, project *model.Project) error
	Delete(ctx context.Context, id uuid.UUID) error
}

type projectRepository struct {
	pool *pgxpool.Pool
}

func NewProjectRepository(pool *pgxpool.Pool) ProjectRepository {
	return &projectRepository{pool: pool}
}

func (r *projectRepository) Create(ctx context.Context, project *model.Project) error {
	query := `
		INSERT INTO projects (id, org_id, name, description, status, priority, owner_id, due_date, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		project.ID, project.OrgID, project.Name, project.Description,
		project.Status, project.Priority, project.OwnerID, project.DueDate,
	).Scan(&project.CreatedAt, &project.UpdatedAt)
}

func (r *projectRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Project, error) {
	query := `
		SELECT id, org_id, name, description, status, priority, owner_id, due_date, created_at, updated_at
		FROM projects WHERE id = $1`

	p := &model.Project{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&p.ID, &p.OrgID, &p.Name, &p.Description, &p.Status, &p.Priority,
		&p.OwnerID, &p.DueDate, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding project by id: %w", err)
	}
	return p, nil
}

func (r *projectRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Project], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM projects WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting projects: %w", err)
	}

	query := `
		SELECT id, org_id, name, description, status, priority, owner_id, due_date, created_at, updated_at
		FROM projects WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("listing projects: %w", err)
	}
	defer rows.Close()

	projects := make([]model.Project, 0)
	for rows.Next() {
		var p model.Project
		if err := rows.Scan(
			&p.ID, &p.OrgID, &p.Name, &p.Description, &p.Status, &p.Priority,
			&p.OwnerID, &p.DueDate, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning project: %w", err)
		}
		projects = append(projects, p)
	}

	totalPages := total / params.PerPage
	if total%params.PerPage != 0 {
		totalPages++
	}

	return &model.PaginatedResponse[model.Project]{
		Data: projects, Total: total, Page: params.Page,
		PerPage: params.PerPage, TotalPages: totalPages,
	}, nil
}

func (r *projectRepository) Update(ctx context.Context, project *model.Project) error {
	query := `
		UPDATE projects SET name = $1, description = $2, status = $3, priority = $4,
			due_date = $5, updated_at = NOW()
		WHERE id = $6
		RETURNING updated_at`

	return r.pool.QueryRow(ctx, query,
		project.Name, project.Description, project.Status, project.Priority,
		project.DueDate, project.ID,
	).Scan(&project.UpdatedAt)
}

func (r *projectRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, id)
	return err
}

