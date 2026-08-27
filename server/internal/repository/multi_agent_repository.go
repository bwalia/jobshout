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
	// An agent's board column comes from its most recent activity, where
	// "activity" has two sources:
	//
	//   1. multi_agent_jobs — the agent participated as planner/executor/reviewer.
	//   2. blog_runs        — the agent is the article generator for that run.
	//   3. mail_threads     — Mail Agent work, plus Research Agent while a
	//                       thread is in researching so the board does not lie.
	//
	// All are normalised into the same shape, unioned, and the newest row per
	// agent wins. Agents with no activity at all fall through to idle.
	//
	// For blog runs the running step is pulled out of the steps JSONB so the
	// card can name the exact phase the agent is on, rather than just "running".
	const sql = `
		WITH job_activity AS (
			SELECT a.id AS agent_id,
			       'job'::text     AS kind,
			       j.id            AS source_id,
			       j.status::text  AS status,
			       NULL::text      AS step_key,
			       j.task_prompt::text AS detail,
			       CASE
			           WHEN a.id = j.planner_id  THEN 'planner'
			           WHEN a.id = j.executor_id THEN 'executor'
			           WHEN a.id = j.reviewer_id THEN 'reviewer'
			       END::text       AS job_role,
			       j.created_at
			FROM agents a
			JOIN multi_agent_jobs j
			       ON j.org_id = a.org_id
			      AND (a.id = j.planner_id OR a.id = j.executor_id OR a.id = j.reviewer_id)
			WHERE a.org_id = $1
		),
		blog_activity AS (
			SELECT b.agent_id,
			       'blog'::text    AS kind,
			       b.id            AS source_id,
			       b.status::text  AS status,
			       s.key::text     AS step_key,
			       COALESCE(s.label, b.status)::text AS detail,
			       'writer'::text  AS job_role,
			       b.created_at
			FROM blog_runs b
			LEFT JOIN LATERAL (
			    SELECT e->>'key' AS key, e->>'label' AS label
			    FROM jsonb_array_elements(b.steps) e
			    WHERE e->>'status' = 'running'
			    LIMIT 1
			) s ON TRUE
			WHERE b.org_id = $1 AND b.agent_id IS NOT NULL
		),
		mail_activity AS (
			SELECT t.agent_id,
			       'mail'::text    AS kind,
			       t.id            AS source_id,
			       t.status::text  AS status,
			       NULL::text      AS step_key,
			       COALESCE(NULLIF(t.subject, ''), t.status)::text AS detail,
			       'mail'::text    AS job_role,
			       t.updated_at    AS created_at
			FROM mail_threads t
			WHERE t.org_id = $1 AND t.agent_id IS NOT NULL
		),
		mail_research_activity AS (
			SELECT a.id            AS agent_id,
			       'mail'::text    AS kind,
			       t.id            AS source_id,
			       t.status::text  AS status,
			       'research'::text AS step_key,
			       COALESCE(NULLIF(t.subject, ''), 'researching email')::text AS detail,
			       'researcher'::text AS job_role,
			       t.updated_at    AS created_at
			FROM mail_threads t
			JOIN agents a
			       ON a.org_id = t.org_id
			      AND a.metadata->>'builtin' = 'researcher'
			WHERE t.org_id = $1 AND t.status = 'researching'
		),
		combined AS (
			SELECT u.*, ROW_NUMBER() OVER (PARTITION BY u.agent_id ORDER BY u.created_at DESC) AS rn
			FROM (
				SELECT * FROM job_activity
				UNION ALL SELECT * FROM blog_activity
				UNION ALL SELECT * FROM mail_activity
				UNION ALL SELECT * FROM mail_research_activity
			) u
		)
		SELECT a.id, a.name, a.role, a.avatar_url,
		       c.kind, c.source_id, c.status, c.step_key, c.detail, c.job_role, c.created_at
		FROM agents a
		LEFT JOIN combined c ON c.agent_id = a.id AND c.rn = 1
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
			kind      *string
			sourceID  *uuid.UUID
			status    *string
			stepKey   *string
			detail    *string
			role      *string
			createdAt *time.Time
		)
		if err := rows.Scan(&e.AgentID, &e.Name, &e.Role, &e.AvatarURL,
			&kind, &sourceID, &status, &stepKey, &detail, &role, &createdAt); err != nil {
			return nil, fmt.Errorf("multi_agent_repo: board scan: %w", err)
		}
		e.Activity = boardActivity(kind, status, stepKey)
		if e.Activity != model.ActivityIdle {
			e.CurrentJobID = sourceID
			e.JobRole = role
			e.CurrentJobPrompt = detail
		}
		e.LastActiveAt = createdAt
		out = append(out, e)
	}
	return out, rows.Err()
}

// boardActivity maps one normalised activity row to an agent-board column.
// Terminal states map to idle so a finished job frees the agent up.
//
// Blog runs additionally consult the running step: generation and conversion
// are ordinary work (executing), while the publishing phase gets its own column
// because sending an article to the CMS is materially different from writing
// one here.
func boardActivity(kind, status, stepKey *string) string {
	if kind == nil || status == nil {
		return model.ActivityIdle
	}

	switch *kind {
	case "job":
		switch *status {
		case model.MultiAgentStatusPlanning:
			return model.ActivityPlanning
		case model.MultiAgentStatusExecuting:
			return model.ActivityExecuting
		case model.MultiAgentStatusReviewing:
			return model.ActivityReviewing
		case model.MultiAgentStatusFailed:
			return model.ActivityFailed
		}
		return model.ActivityIdle

	case "blog":
		switch *status {
		case model.BlogRunStatusFailed:
			return model.ActivityFailed
		case model.BlogRunStatusRunning:
			if stepKey != nil {
				switch *stepKey {
				case model.BlogStepPublishing:
					return model.ActivityPublishing
				}
			}
			return model.ActivityExecuting
		}
		return model.ActivityIdle

	case "mail":
		switch *status {
		case model.MailThreadFailed:
			return model.ActivityFailed
		case model.MailThreadDraftReady:
			return model.ActivityReviewing
		case model.MailThreadNew, model.MailThreadClassifying, model.MailThreadResearching:
			return model.ActivityExecuting
		}
		return model.ActivityIdle
	}

	return model.ActivityIdle
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
