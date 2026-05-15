package repository

import (
	"context"
	"fmt"
	"time"

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
	// BoardEntries returns one row per agent in the org with its current
	// activity derived from the most recent multi_agent_jobs participation
	// (LATERAL pick of the latest job touching the agent in any role).
	BoardEntries(ctx context.Context, orgID uuid.UUID) ([]model.AgentBoardEntry, error)
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

func (r *multiAgentRepository) BoardEntries(ctx context.Context, orgID uuid.UUID) ([]model.AgentBoardEntry, error) {
	// For each agent in the org, look at the latest multi_agent_jobs row that
	// references it (in any of planner/executor/reviewer). Status drives the
	// board column:
	//   - planning  → planner column (job.status = 'planning')
	//   - executing → executor column
	//   - reviewing → reviewer column
	//   - failed    → failed column (terminal)
	//   - completed → idle (treated as "ready for next task")
	// Agents with no jobs at all also land in idle.
	const sql = `
		WITH latest AS (
			SELECT a.id AS agent_id, j.id AS job_id, j.status, j.task_prompt, j.created_at,
			       CASE
			           WHEN a.id = j.planner_id  THEN 'planner'
			           WHEN a.id = j.executor_id THEN 'executor'
			           WHEN a.id = j.reviewer_id THEN 'reviewer'
			           ELSE NULL
			       END AS job_role,
			       ROW_NUMBER() OVER (PARTITION BY a.id ORDER BY j.created_at DESC) AS rn
			FROM agents a
			LEFT JOIN multi_agent_jobs j
			       ON j.org_id = a.org_id
			      AND (a.id = j.planner_id OR a.id = j.executor_id OR a.id = j.reviewer_id)
			WHERE a.org_id = $1
		)
		SELECT a.id, a.name, a.role, a.avatar_url,
		       latest.job_id, latest.status, latest.task_prompt, latest.job_role, latest.created_at
		FROM agents a
		LEFT JOIN latest ON latest.agent_id = a.id AND (latest.rn = 1 OR latest.rn IS NULL)
		WHERE a.org_id = $1
		ORDER BY a.name`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("multi_agent_repo: board: %w", err)
	}
	defer rows.Close()

	out := []model.AgentBoardEntry{}
	for rows.Next() {
		var (
			e         model.AgentBoardEntry
			jobID     *uuid.UUID
			jobStatus *string
			prompt    *string
			role      *string
			createdAt *time.Time
		)
		if err := rows.Scan(&e.AgentID, &e.Name, &e.Role, &e.AvatarURL,
			&jobID, &jobStatus, &prompt, &role, &createdAt); err != nil {
			return nil, fmt.Errorf("multi_agent_repo: board scan: %w", err)
		}
		e.Activity = boardActivity(jobStatus)
		if e.Activity != "idle" {
			e.CurrentJobID = jobID
			e.JobRole = role
			e.CurrentJobPrompt = prompt
		}
		e.LastActiveAt = createdAt
		out = append(out, e)
	}
	return out, rows.Err()
}

// boardActivity maps a multi_agent_jobs.status to the agent-board column.
// Completed → idle so finished jobs free up the agent.
func boardActivity(jobStatus *string) string {
	if jobStatus == nil {
		return "idle"
	}
	switch *jobStatus {
	case model.MultiAgentStatusPlanning:
		return "planning"
	case model.MultiAgentStatusExecuting:
		return "executing"
	case model.MultiAgentStatusReviewing:
		return "reviewing"
	case model.MultiAgentStatusFailed:
		return "failed"
	default:
		return "idle"
	}
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
