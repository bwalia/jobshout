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

type TaskRepository interface {
	Create(ctx context.Context, task *model.Task) error
	FindByID(ctx context.Context, id uuid.UUID) (*model.Task, error)
	ListByProject(ctx context.Context, projectID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error)
	Update(ctx context.Context, task *model.Task) error
	Delete(ctx context.Context, id uuid.UUID) error
	TransitionStatus(ctx context.Context, id uuid.UUID, status string) error
	Reorder(ctx context.Context, id uuid.UUID, status string, position int) error
}

type taskRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRepository(pool *pgxpool.Pool) TaskRepository {
	return &taskRepository{pool: pool}
}

func (r *taskRepository) Create(ctx context.Context, task *model.Task) error {
	// Get next position for the status column
	var maxPos int
	posQuery := `SELECT COALESCE(MAX(position), -1) + 1 FROM tasks WHERE project_id = $1 AND status = $2`
	if err := r.pool.QueryRow(ctx, posQuery, task.ProjectID, task.Status).Scan(&maxPos); err != nil {
		return fmt.Errorf("getting max position: %w", err)
	}
	task.Position = maxPos

	query := `
		INSERT INTO tasks (id, project_id, parent_id, title, description, status, priority,
			assigned_agent_id, assigned_user_id, story_points, due_date, position, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		task.ID, task.ProjectID, task.ParentID, task.Title, task.Description,
		task.Status, task.Priority, task.AssignedAgentID, task.AssignedUserID,
		task.StoryPoints, task.DueDate, task.Position, task.CreatedBy,
	).Scan(&task.CreatedAt, &task.UpdatedAt)
}

func (r *taskRepository) FindByID(ctx context.Context, id uuid.UUID) (*model.Task, error) {
	query := `
		SELECT id, project_id, parent_id, title, description, status, priority,
			assigned_agent_id, assigned_user_id, story_points, due_date, position,
			created_by, created_at, updated_at
		FROM tasks WHERE id = $1`

	t := &model.Task{}
	err := r.pool.QueryRow(ctx, query, id).Scan(
		&t.ID, &t.ProjectID, &t.ParentID, &t.Title, &t.Description,
		&t.Status, &t.Priority, &t.AssignedAgentID, &t.AssignedUserID,
		&t.StoryPoints, &t.DueDate, &t.Position, &t.CreatedBy,
		&t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding task by id: %w", err)
	}
	return t, nil
}

func (r *taskRepository) ListByProject(ctx context.Context, projectID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Task], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM tasks WHERE project_id = $1`, projectID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting tasks: %w", err)
	}

	query := `
		SELECT id, project_id, parent_id, title, description, status, priority,
			assigned_agent_id, assigned_user_id, story_points, due_date, position,
			created_by, created_at, updated_at
		FROM tasks WHERE project_id = $1
		ORDER BY status, position ASC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, query, projectID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("listing tasks: %w", err)
	}
	defer rows.Close()

	tasks := make([]model.Task, 0)
	for rows.Next() {
		var t model.Task
		if err := rows.Scan(
			&t.ID, &t.ProjectID, &t.ParentID, &t.Title, &t.Description,
			&t.Status, &t.Priority, &t.AssignedAgentID, &t.AssignedUserID,
			&t.StoryPoints, &t.DueDate, &t.Position, &t.CreatedBy,
			&t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning task: %w", err)
		}
		tasks = append(tasks, t)
	}

	totalPages := total / params.PerPage
	if total%params.PerPage != 0 {
		totalPages++
	}

	return &model.PaginatedResponse[model.Task]{
		Data: tasks, Total: total, Page: params.Page,
		PerPage: params.PerPage, TotalPages: totalPages,
	}, nil
}

func (r *taskRepository) Update(ctx context.Context, task *model.Task) error {
	query := `
		UPDATE tasks SET title = $1, description = $2, priority = $3,
			assigned_agent_id = $4, assigned_user_id = $5, story_points = $6,
			due_date = $7, updated_at = NOW()
		WHERE id = $8
		RETURNING updated_at`

	return r.pool.QueryRow(ctx, query,
		task.Title, task.Description, task.Priority,
		task.AssignedAgentID, task.AssignedUserID, task.StoryPoints,
		task.DueDate, task.ID,
	).Scan(&task.UpdatedAt)
}

func (r *taskRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM tasks WHERE id = $1`, id)
	return err
}

func (r *taskRepository) TransitionStatus(ctx context.Context, id uuid.UUID, status string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// Get new position at end of target column
	var task model.Task
	if err := tx.QueryRow(ctx, `SELECT project_id FROM tasks WHERE id = $1`, id).Scan(&task.ProjectID); err != nil {
		return fmt.Errorf("finding task: %w", err)
	}

	var maxPos int
	if err := tx.QueryRow(ctx,
		`SELECT COALESCE(MAX(position), -1) + 1 FROM tasks WHERE project_id = $1 AND status = $2`,
		task.ProjectID, status,
	).Scan(&maxPos); err != nil {
		return fmt.Errorf("getting max position: %w", err)
	}

	_, err = tx.Exec(ctx,
		`UPDATE tasks SET status = $1, position = $2, updated_at = NOW() WHERE id = $3`,
		status, maxPos, id,
	)
	if err != nil {
		return fmt.Errorf("updating task status: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *taskRepository) Reorder(ctx context.Context, id uuid.UUID, status string, position int) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var projectID uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT project_id FROM tasks WHERE id = $1 FOR UPDATE`, id).Scan(&projectID); err != nil {
		return fmt.Errorf("finding task for reorder: %w", err)
	}

	// Shift tasks at target position and after
	_, err = tx.Exec(ctx,
		`UPDATE tasks SET position = position + 1 WHERE project_id = $1 AND status = $2 AND position >= $3 AND id != $4`,
		projectID, status, position, id,
	)
	if err != nil {
		return fmt.Errorf("shifting tasks: %w", err)
	}

	// Place the task at the target position
	_, err = tx.Exec(ctx,
		`UPDATE tasks SET status = $1, position = $2, updated_at = NOW() WHERE id = $3`,
		status, position, id,
	)
	if err != nil {
		return fmt.Errorf("placing task: %w", err)
	}

	return tx.Commit(ctx)
}
