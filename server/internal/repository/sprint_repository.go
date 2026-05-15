package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// SprintRepository persists sprints and their job/agent associations.
type SprintRepository interface {
	Create(ctx context.Context, s *model.Sprint) error
	Get(ctx context.Context, id uuid.UUID) (*model.Sprint, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.Sprint, error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateSprintRequest) (*model.Sprint, error)
	Delete(ctx context.Context, id uuid.UUID) error

	AddJob(ctx context.Context, sprintID, jobID uuid.UUID, position int) error
	RemoveJob(ctx context.Context, sprintID, jobID uuid.UUID) error
	ListJobs(ctx context.Context, sprintID uuid.UUID) ([]model.MultiAgentJob, error)

	AddAgent(ctx context.Context, sprintID, agentID uuid.UUID, roleLabel string) error
	RemoveAgent(ctx context.Context, sprintID, agentID uuid.UUID, roleLabel string) error
	ListAgents(ctx context.Context, sprintID uuid.UUID) ([]model.SprintAgentInfo, error)
}

type sprintRepository struct {
	pool *pgxpool.Pool
}

func NewSprintRepository(pool *pgxpool.Pool) SprintRepository {
	return &sprintRepository{pool: pool}
}

const sprintColumns = `id, org_id, name, goal, status, start_at, end_at, velocity, created_by, created_at, updated_at`

func scanSprint(s rowScanner, out *model.Sprint) error {
	return s.Scan(
		&out.ID, &out.OrgID, &out.Name, &out.Goal, &out.Status,
		&out.StartAt, &out.EndAt, &out.Velocity, &out.CreatedBy,
		&out.CreatedAt, &out.UpdatedAt,
	)
}

func (r *sprintRepository) Create(ctx context.Context, s *model.Sprint) error {
	const sql = `
		INSERT INTO sprints (id, org_id, name, goal, status, start_at, end_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING created_at, updated_at`

	if s.Status == "" {
		s.Status = model.SprintStatusPlanning
	}
	return r.pool.QueryRow(ctx, sql,
		s.ID, s.OrgID, s.Name, s.Goal, s.Status, s.StartAt, s.EndAt, s.CreatedBy,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
}

func (r *sprintRepository) Get(ctx context.Context, id uuid.UUID) (*model.Sprint, error) {
	sql := `SELECT ` + sprintColumns + ` FROM sprints WHERE id = $1`
	out := &model.Sprint{}
	if err := scanSprint(r.pool.QueryRow(ctx, sql, id), out); err != nil {
		return nil, fmt.Errorf("sprint_repo: get: %w", err)
	}
	return out, nil
}

func (r *sprintRepository) ListByOrg(ctx context.Context, orgID uuid.UUID) ([]model.Sprint, error) {
	sql := `SELECT ` + sprintColumns + ` FROM sprints
		WHERE org_id = $1
		ORDER BY
		  -- Active first, then planning, then completed/cancelled — matches the UX
		  -- where a user lands on the active sprint by default.
		  CASE status
		    WHEN 'active'   THEN 0
		    WHEN 'planning' THEN 1
		    WHEN 'completed' THEN 2
		    WHEN 'cancelled' THEN 3
		    ELSE 4
		  END,
		  COALESCE(start_at, created_at) DESC`

	rows, err := r.pool.Query(ctx, sql, orgID)
	if err != nil {
		return nil, fmt.Errorf("sprint_repo: list: %w", err)
	}
	defer rows.Close()

	out := []model.Sprint{}
	for rows.Next() {
		var s model.Sprint
		if err := scanSprint(rows, &s); err != nil {
			return nil, fmt.Errorf("sprint_repo: list scan: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *sprintRepository) Update(ctx context.Context, id uuid.UUID, req model.UpdateSprintRequest) (*model.Sprint, error) {
	const sql = `
		UPDATE sprints SET
		    name     = COALESCE($2, name),
		    goal     = COALESCE($3, goal),
		    status   = COALESCE($4, status),
		    start_at = COALESCE($5, start_at),
		    end_at   = COALESCE($6, end_at),
		    updated_at = NOW()
		WHERE id = $1`

	if _, err := r.pool.Exec(ctx, sql, id, req.Name, req.Goal, req.Status, req.StartAt, req.EndAt); err != nil {
		return nil, fmt.Errorf("sprint_repo: update: %w", err)
	}
	return r.Get(ctx, id)
}

func (r *sprintRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sprints WHERE id = $1`, id)
	return err
}

func (r *sprintRepository) AddJob(ctx context.Context, sprintID, jobID uuid.UUID, position int) error {
	const sql = `
		INSERT INTO sprint_jobs (sprint_id, job_id, position)
		VALUES ($1, $2, $3)
		ON CONFLICT (sprint_id, job_id) DO UPDATE SET position = EXCLUDED.position`
	_, err := r.pool.Exec(ctx, sql, sprintID, jobID, position)
	return err
}

func (r *sprintRepository) RemoveJob(ctx context.Context, sprintID, jobID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM sprint_jobs WHERE sprint_id = $1 AND job_id = $2`, sprintID, jobID)
	return err
}

func (r *sprintRepository) ListJobs(ctx context.Context, sprintID uuid.UUID) ([]model.MultiAgentJob, error) {
	const sql = `
		SELECT j.id, j.org_id, j.task_prompt, j.planner_id, j.executor_id, j.reviewer_id,
		       j.status, j.plan_output, j.exec_output, j.review_output, j.approved,
		       j.iterations, j.max_review, j.error_msg, j.created_at, j.completed_at
		FROM multi_agent_jobs j
		JOIN sprint_jobs sj ON sj.job_id = j.id
		WHERE sj.sprint_id = $1
		ORDER BY sj.position, j.created_at`

	rows, err := r.pool.Query(ctx, sql, sprintID)
	if err != nil {
		return nil, fmt.Errorf("sprint_repo: list jobs: %w", err)
	}
	defer rows.Close()

	jobs := []model.MultiAgentJob{}
	for rows.Next() {
		var j model.MultiAgentJob
		if err := rows.Scan(
			&j.ID, &j.OrgID, &j.TaskPrompt, &j.PlannerID, &j.ExecutorID, &j.ReviewerID,
			&j.Status, &j.PlanOutput, &j.ExecOutput, &j.ReviewOutput, &j.Approved,
			&j.Iterations, &j.MaxReview, &j.ErrorMsg, &j.CreatedAt, &j.CompletedAt,
		); err != nil {
			return nil, fmt.Errorf("sprint_repo: list jobs scan: %w", err)
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

func (r *sprintRepository) AddAgent(ctx context.Context, sprintID, agentID uuid.UUID, roleLabel string) error {
	if roleLabel == "" {
		roleLabel = "any"
	}
	const sql = `
		INSERT INTO sprint_agents (sprint_id, agent_id, role_label)
		VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`
	_, err := r.pool.Exec(ctx, sql, sprintID, agentID, roleLabel)
	return err
}

func (r *sprintRepository) RemoveAgent(ctx context.Context, sprintID, agentID uuid.UUID, roleLabel string) error {
	const sql = `DELETE FROM sprint_agents WHERE sprint_id = $1 AND agent_id = $2 AND role_label = $3`
	_, err := r.pool.Exec(ctx, sql, sprintID, agentID, roleLabel)
	return err
}

func (r *sprintRepository) ListAgents(ctx context.Context, sprintID uuid.UUID) ([]model.SprintAgentInfo, error) {
	const sql = `
		SELECT a.id, a.name, a.role, a.avatar_url, sa.role_label
		FROM sprint_agents sa
		JOIN agents a ON a.id = sa.agent_id
		WHERE sa.sprint_id = $1
		ORDER BY a.name`

	rows, err := r.pool.Query(ctx, sql, sprintID)
	if err != nil {
		return nil, fmt.Errorf("sprint_repo: list agents: %w", err)
	}
	defer rows.Close()

	out := []model.SprintAgentInfo{}
	for rows.Next() {
		var a model.SprintAgentInfo
		if err := rows.Scan(&a.AgentID, &a.Name, &a.Role, &a.AvatarURL, &a.RoleLabel); err != nil {
			return nil, fmt.Errorf("sprint_repo: list agents scan: %w", err)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}
