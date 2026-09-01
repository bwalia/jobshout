package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// TaskRunRepository persists on-demand task runs.
type TaskRunRepository interface {
	Create(ctx context.Context, run *model.TaskRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.TaskRun, error)
	Update(ctx context.Context, run *model.TaskRun) error
	ListByTask(ctx context.Context, taskID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.TaskRun], error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.TaskRun], error)
	// CountActiveByTask is queued+running runs for a board card. Used so a
	// finished run does not mark the card Done while another run is still live.
	CountActiveByTask(ctx context.Context, taskID uuid.UUID) (int, error)
}

type taskRunRepository struct {
	pool *pgxpool.Pool
}

func NewTaskRunRepository(pool *pgxpool.Pool) TaskRunRepository {
	return &taskRunRepository{pool: pool}
}

const taskRunColumns = `
	id, task_id, agent_id, org_id, execution_id, status, prompt, engine,
	model_provider, model_name, skill_slugs, inputs, debug, output, error_message,
	total_tokens, cost_usd, latency_ms, iterations, requested_by,
	started_at, completed_at, created_at, updated_at`

func scanTaskRun(row pgx.Row) (*model.TaskRun, error) {
	run := &model.TaskRun{}
	var inputsRaw []byte
	if err := row.Scan(
		&run.ID, &run.TaskID, &run.AgentID, &run.OrgID, &run.ExecutionID, &run.Status,
		&run.Prompt, &run.Engine, &run.ModelProvider, &run.ModelName, &run.SkillSlugs,
		&inputsRaw, &run.Debug, &run.Output, &run.ErrorMessage,
		&run.TotalTokens, &run.CostUSD, &run.LatencyMs, &run.Iterations, &run.RequestedBy,
		&run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(inputsRaw) > 0 {
		_ = json.Unmarshal(inputsRaw, &run.Inputs)
	}
	return run, nil
}

func (r *taskRunRepository) Create(ctx context.Context, run *model.TaskRun) error {
	inputs, err := marshalInputs(run.Inputs)
	if err != nil {
		return err
	}
	if run.SkillSlugs == nil {
		run.SkillSlugs = []string{}
	}
	query := `
		INSERT INTO task_runs (
			id, task_id, agent_id, org_id, execution_id, status, prompt, engine,
			model_provider, model_name, skill_slugs, inputs, debug, output, error_message,
			total_tokens, cost_usd, latency_ms, iterations, requested_by,
			started_at, completed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15,
			$16, $17, $18, $19, $20, $21, $22, NOW(), NOW())
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		run.ID, run.TaskID, run.AgentID, run.OrgID, run.ExecutionID, run.Status,
		run.Prompt, run.Engine, run.ModelProvider, run.ModelName, run.SkillSlugs,
		inputs, run.Debug, run.Output, run.ErrorMessage,
		run.TotalTokens, run.CostUSD, run.LatencyMs, run.Iterations, run.RequestedBy,
		run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *taskRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.TaskRun, error) {
	query := `SELECT ` + taskRunColumns + ` FROM task_runs WHERE id = $1`
	run, err := scanTaskRun(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("task run not found")
		}
		return nil, fmt.Errorf("finding task run: %w", err)
	}
	return run, nil
}

func (r *taskRunRepository) Update(ctx context.Context, run *model.TaskRun) error {
	inputs, err := marshalInputs(run.Inputs)
	if err != nil {
		return err
	}
	if run.SkillSlugs == nil {
		run.SkillSlugs = []string{}
	}
	query := `
		UPDATE task_runs
		SET execution_id = $2, status = $3, prompt = $4, engine = $5,
		    model_provider = $6, model_name = $7, skill_slugs = $8, inputs = $9,
		    debug = $10, output = $11, error_message = $12, total_tokens = $13,
		    cost_usd = $14, latency_ms = $15, iterations = $16,
		    started_at = $17, completed_at = $18, updated_at = NOW()
		WHERE id = $1
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, query,
		run.ID, run.ExecutionID, run.Status, run.Prompt, run.Engine,
		run.ModelProvider, run.ModelName, run.SkillSlugs, inputs,
		run.Debug, run.Output, run.ErrorMessage, run.TotalTokens,
		run.CostUSD, run.LatencyMs, run.Iterations,
		run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *taskRunRepository) CountActiveByTask(ctx context.Context, taskID uuid.UUID) (int, error) {
	var n int
	err := r.pool.QueryRow(ctx, `
		SELECT COUNT(*) FROM task_runs
		WHERE task_id = $1 AND status IN ('queued', 'running')`, taskID).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("counting active task runs: %w", err)
	}
	return n, nil
}

func (r *taskRunRepository) ListByTask(ctx context.Context, taskID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.TaskRun], error) {
	pagination.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_runs WHERE task_id = $1`, taskID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting task runs: %w", err)
	}

	query := `SELECT ` + taskRunColumns + `
		FROM task_runs WHERE task_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	return r.listPaged(ctx, query, taskID, pagination, total)
}

func (r *taskRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.TaskRun], error) {
	pagination.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM task_runs WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting task runs: %w", err)
	}

	query := `SELECT ` + taskRunColumns + `
		FROM task_runs WHERE org_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`
	return r.listPaged(ctx, query, orgID, pagination, total)
}

func (r *taskRunRepository) listPaged(ctx context.Context, query string, scope uuid.UUID, pagination model.PaginationParams, total int) (*model.PaginatedResponse[model.TaskRun], error) {
	rows, err := r.pool.Query(ctx, query, scope, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, fmt.Errorf("listing task runs: %w", err)
	}
	defer rows.Close()

	runs := make([]model.TaskRun, 0, pagination.PerPage)
	for rows.Next() {
		run, err := scanTaskRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning task run: %w", err)
		}
		runs = append(runs, *run)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &model.PaginatedResponse[model.TaskRun]{
		Data:       runs,
		Total:      total,
		Page:       pagination.Page,
		PerPage:    pagination.PerPage,
		TotalPages: (total + pagination.PerPage - 1) / pagination.PerPage,
	}, nil
}

// marshalInputs renders the free-form inputs map as JSONB bytes, defaulting to
// an empty object so the NOT NULL column is always satisfied.
func marshalInputs(inputs map[string]any) ([]byte, error) {
	if len(inputs) == 0 {
		return []byte("{}"), nil
	}
	b, err := json.Marshal(inputs)
	if err != nil {
		return nil, fmt.Errorf("marshalling task run inputs: %w", err)
	}
	return b, nil
}
