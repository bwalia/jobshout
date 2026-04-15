package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// MultiAgentRepository manages persistence for multi-agent collaboration jobs.
type MultiAgentRepository interface {
	Create(ctx context.Context, job *model.MultiAgentJob) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.MultiAgentJob, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdatePlanOutput(ctx context.Context, id uuid.UUID, output string) error
	UpdateExecOutput(ctx context.Context, id uuid.UUID, output string) error
	UpdateReviewOutput(ctx context.Context, id uuid.UUID, output string, approved bool) error
	IncrementIteration(ctx context.Context, id uuid.UUID) error
	MarkCompleted(ctx context.Context, id uuid.UUID) error
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.MultiAgentJob], error)
}

type multiAgentRepository struct {
	pool *pgxpool.Pool
}

func NewMultiAgentRepository(pool *pgxpool.Pool) MultiAgentRepository {
	return &multiAgentRepository{pool: pool}
}

func (r *multiAgentRepository) Create(ctx context.Context, job *model.MultiAgentJob) error {
	if job.ID == uuid.Nil {
		job.ID = uuid.New()
	}
	const sql = `
		INSERT INTO multi_agent_jobs (id, org_id, task_prompt, planner_id, executor_id, reviewer_id, status, max_review)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err := r.pool.Exec(ctx, sql,
		job.ID, job.OrgID, job.TaskPrompt,
		job.PlannerID, job.ExecutorID, job.ReviewerID,
		job.Status, job.MaxReview,
	)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: create: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.MultiAgentJob, error) {
	const sql = `
		SELECT id, org_id, task_prompt, planner_id, executor_id, reviewer_id,
		       status, plan_output, exec_output, review_output, approved,
		       iterations, max_review, error_msg, created_at, completed_at
		FROM multi_agent_jobs WHERE id = $1`

	var j model.MultiAgentJob
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&j.ID, &j.OrgID, &j.TaskPrompt,
		&j.PlannerID, &j.ExecutorID, &j.ReviewerID,
		&j.Status, &j.PlanOutput, &j.ExecOutput, &j.ReviewOutput, &j.Approved,
		&j.Iterations, &j.MaxReview, &j.ErrorMsg, &j.CreatedAt, &j.CompletedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("multi_agent_repo: get_by_id: %w", err)
	}
	return &j, nil
}

func (r *multiAgentRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	const sql = `UPDATE multi_agent_jobs SET status = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, status)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: update_status: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) UpdatePlanOutput(ctx context.Context, id uuid.UUID, output string) error {
	const sql = `UPDATE multi_agent_jobs SET plan_output = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, output)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: update_plan_output: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) UpdateExecOutput(ctx context.Context, id uuid.UUID, output string) error {
	const sql = `UPDATE multi_agent_jobs SET exec_output = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, output)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: update_exec_output: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) UpdateReviewOutput(ctx context.Context, id uuid.UUID, output string, approved bool) error {
	const sql = `UPDATE multi_agent_jobs SET review_output = $2, approved = $3 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, output, approved)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: update_review_output: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) IncrementIteration(ctx context.Context, id uuid.UUID) error {
	const sql = `UPDATE multi_agent_jobs SET iterations = iterations + 1 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: increment_iteration: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) MarkCompleted(ctx context.Context, id uuid.UUID) error {
	const sql = `UPDATE multi_agent_jobs SET status = 'completed', completed_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: mark_completed: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	const sql = `UPDATE multi_agent_jobs SET status = 'failed', error_msg = $2, completed_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, errMsg)
	if err != nil {
		return fmt.Errorf("multi_agent_repo: mark_failed: %w", err)
	}
	return nil
}

func (r *multiAgentRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.MultiAgentJob], error) {
	params.Normalize()

	const countSQL = `SELECT COUNT(*) FROM multi_agent_jobs WHERE org_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("multi_agent_repo: list count: %w", err)
	}

	const sql = `
		SELECT id, org_id, task_prompt, planner_id, executor_id, reviewer_id,
		       status, plan_output, exec_output, review_output, approved,
		       iterations, max_review, error_msg, created_at, completed_at
		FROM multi_agent_jobs
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("multi_agent_repo: list: %w", err)
	}
	defer rows.Close()

	var jobs []model.MultiAgentJob
	for rows.Next() {
		var j model.MultiAgentJob
		if err := rows.Scan(
			&j.ID, &j.OrgID, &j.TaskPrompt,
			&j.PlannerID, &j.ExecutorID, &j.ReviewerID,
			&j.Status, &j.PlanOutput, &j.ExecOutput, &j.ReviewOutput, &j.Approved,
			&j.Iterations, &j.MaxReview, &j.ErrorMsg, &j.CreatedAt, &j.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("multi_agent_repo: list scan: %w", err)
		}
		jobs = append(jobs, j)
	}

	return &model.PaginatedResponse[model.MultiAgentJob]{
		Data:    jobs,
		Total:   total,
		Page:    params.Page,
		PerPage: params.PerPage,
	}, nil
}
