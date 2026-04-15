package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// GoalRepository manages persistence for the autonomous agent goal lifecycle.
type GoalRepository interface {
	Create(ctx context.Context, goal *model.AgentGoal) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.AgentGoal, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
	UpdatePlan(ctx context.Context, id uuid.UUID, plan []model.PlanStep) error
	UpdateReflection(ctx context.Context, id uuid.UUID, reflection string) error
	MarkStarted(ctx context.Context, id uuid.UUID) error
	MarkCompleted(ctx context.Context, id uuid.UUID, reflection string) error
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error
	IncrementIteration(ctx context.Context, id uuid.UUID) error
	ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentGoal], error)
}

type goalRepository struct {
	pool *pgxpool.Pool
}

func NewGoalRepository(pool *pgxpool.Pool) GoalRepository {
	return &goalRepository{pool: pool}
}

func (r *goalRepository) Create(ctx context.Context, goal *model.AgentGoal) error {
	if goal.ID == uuid.Nil {
		goal.ID = uuid.New()
	}
	planJSON, err := json.Marshal(goal.Plan)
	if err != nil {
		return fmt.Errorf("goal_repo: marshal plan: %w", err)
	}
	const sql = `
		INSERT INTO agent_goals (id, agent_id, org_id, session_id, goal_text, plan, status, max_iter)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

	_, err = r.pool.Exec(ctx, sql,
		goal.ID, goal.AgentID, goal.OrgID, goal.SessionID,
		goal.GoalText, planJSON, goal.Status, goal.MaxIter,
	)
	if err != nil {
		return fmt.Errorf("goal_repo: create: %w", err)
	}
	return nil
}

func (r *goalRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.AgentGoal, error) {
	const sql = `
		SELECT id, agent_id, org_id, session_id, goal_text, plan, status,
		       reflection, iterations, max_iter, error_msg,
		       started_at, completed_at, created_at
		FROM agent_goals WHERE id = $1`

	var g model.AgentGoal
	var planJSON []byte
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&g.ID, &g.AgentID, &g.OrgID, &g.SessionID, &g.GoalText,
		&planJSON, &g.Status, &g.Reflection, &g.Iterations, &g.MaxIter,
		&g.ErrorMsg, &g.StartedAt, &g.CompletedAt, &g.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("goal_repo: get_by_id: %w", err)
	}
	if err := json.Unmarshal(planJSON, &g.Plan); err != nil {
		g.Plan = []model.PlanStep{}
	}
	return &g, nil
}

func (r *goalRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	const sql = `UPDATE agent_goals SET status = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, status)
	if err != nil {
		return fmt.Errorf("goal_repo: update_status: %w", err)
	}
	return nil
}

func (r *goalRepository) UpdatePlan(ctx context.Context, id uuid.UUID, plan []model.PlanStep) error {
	planJSON, err := json.Marshal(plan)
	if err != nil {
		return fmt.Errorf("goal_repo: marshal plan: %w", err)
	}
	const sql = `UPDATE agent_goals SET plan = $2 WHERE id = $1`
	_, err = r.pool.Exec(ctx, sql, id, planJSON)
	if err != nil {
		return fmt.Errorf("goal_repo: update_plan: %w", err)
	}
	return nil
}

func (r *goalRepository) UpdateReflection(ctx context.Context, id uuid.UUID, reflection string) error {
	const sql = `UPDATE agent_goals SET reflection = $2 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, reflection)
	if err != nil {
		return fmt.Errorf("goal_repo: update_reflection: %w", err)
	}
	return nil
}

func (r *goalRepository) MarkStarted(ctx context.Context, id uuid.UUID) error {
	const sql = `UPDATE agent_goals SET status = 'executing', started_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("goal_repo: mark_started: %w", err)
	}
	return nil
}

func (r *goalRepository) MarkCompleted(ctx context.Context, id uuid.UUID, reflection string) error {
	const sql = `UPDATE agent_goals SET status = 'completed', reflection = $2, completed_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, reflection)
	if err != nil {
		return fmt.Errorf("goal_repo: mark_completed: %w", err)
	}
	return nil
}

func (r *goalRepository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string) error {
	const sql = `UPDATE agent_goals SET status = 'failed', error_msg = $2, completed_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, errMsg)
	if err != nil {
		return fmt.Errorf("goal_repo: mark_failed: %w", err)
	}
	return nil
}

func (r *goalRepository) IncrementIteration(ctx context.Context, id uuid.UUID) error {
	const sql = `UPDATE agent_goals SET iterations = iterations + 1 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id)
	if err != nil {
		return fmt.Errorf("goal_repo: increment_iteration: %w", err)
	}
	return nil
}

func (r *goalRepository) ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentGoal], error) {
	params.Normalize()

	const countSQL = `SELECT COUNT(*) FROM agent_goals WHERE agent_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, agentID).Scan(&total); err != nil {
		return nil, fmt.Errorf("goal_repo: list count: %w", err)
	}

	const sql = `
		SELECT id, agent_id, org_id, session_id, goal_text, plan, status,
		       reflection, iterations, max_iter, error_msg,
		       started_at, completed_at, created_at
		FROM agent_goals
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, agentID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("goal_repo: list: %w", err)
	}
	defer rows.Close()

	var goals []model.AgentGoal
	for rows.Next() {
		var g model.AgentGoal
		var planJSON []byte
		if err := rows.Scan(
			&g.ID, &g.AgentID, &g.OrgID, &g.SessionID, &g.GoalText,
			&planJSON, &g.Status, &g.Reflection, &g.Iterations, &g.MaxIter,
			&g.ErrorMsg, &g.StartedAt, &g.CompletedAt, &g.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("goal_repo: list scan: %w", err)
		}
		if err := json.Unmarshal(planJSON, &g.Plan); err != nil {
			g.Plan = []model.PlanStep{}
		}
		goals = append(goals, g)
	}

	return &model.PaginatedResponse[model.AgentGoal]{
		Data:    goals,
		Total:   total,
		Page:    params.Page,
		PerPage: params.PerPage,
	}, nil
}
