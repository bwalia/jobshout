package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

type SchedulerRepository interface {
	CreateTask(ctx context.Context, t *model.ScheduledTask) error
	GetTask(ctx context.Context, id uuid.UUID) (*model.ScheduledTask, error)
	ListTasks(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ScheduledTask], error)
	UpdateTask(ctx context.Context, id uuid.UUID, req model.UpdateScheduledTaskRequest) (*model.ScheduledTask, error)
	DeleteTask(ctx context.Context, id uuid.UUID) error
	ListDueTasks(ctx context.Context) ([]model.ScheduledTask, error)
	IncrementRunCount(ctx context.Context, id uuid.UUID) error
	CreateRun(ctx context.Context, run *model.ScheduledTaskRun) error
	ListRuns(ctx context.Context, taskID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ScheduledTaskRun], error)
}

type schedulerRepository struct {
	pool *pgxpool.Pool
}

func NewSchedulerRepository(pool *pgxpool.Pool) SchedulerRepository {
	return &schedulerRepository{pool: pool}
}

func (r *schedulerRepository) CreateTask(ctx context.Context, t *model.ScheduledTask) error {
	inputJSON, _ := json.Marshal(t.InputJSON)
	if inputJSON == nil {
		inputJSON = []byte("{}")
	}

	const sql = `
		INSERT INTO scheduled_tasks
		    (id, org_id, name, description, task_type, agent_id, workflow_id,
		     input_prompt, input_json, provider_config_id, model_override,
		     schedule_type, cron_expression, interval_seconds, run_at,
		     status, max_runs, retry_on_failure, max_retries, priority, tags, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22)
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, sql,
		t.ID, t.OrgID, t.Name, t.Description, t.TaskType, t.AgentID, t.WorkflowID,
		t.InputPrompt, inputJSON, t.ProviderConfigID, t.ModelOverride,
		t.ScheduleType, t.CronExpression, t.IntervalSeconds, t.RunAt,
		t.Status, t.MaxRuns, t.RetryOnFailure, t.MaxRetries, t.Priority, t.Tags, t.CreatedBy,
	).Scan(&t.CreatedAt, &t.UpdatedAt)
}

func (r *schedulerRepository) GetTask(ctx context.Context, id uuid.UUID) (*model.ScheduledTask, error) {
	const sql = `
		SELECT id, org_id, name, description, task_type, agent_id, workflow_id,
		       input_prompt, input_json, provider_config_id, model_override,
		       schedule_type, cron_expression, interval_seconds, run_at,
		       status, last_run_at, next_run_at, run_count, max_runs,
		       retry_on_failure, max_retries, priority, tags, created_by, created_at, updated_at
		FROM scheduled_tasks WHERE id = $1`

	t := &model.ScheduledTask{}
	var inputRaw []byte
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&t.ID, &t.OrgID, &t.Name, &t.Description, &t.TaskType, &t.AgentID, &t.WorkflowID,
		&t.InputPrompt, &inputRaw, &t.ProviderConfigID, &t.ModelOverride,
		&t.ScheduleType, &t.CronExpression, &t.IntervalSeconds, &t.RunAt,
		&t.Status, &t.LastRunAt, &t.NextRunAt, &t.RunCount, &t.MaxRuns,
		&t.RetryOnFailure, &t.MaxRetries, &t.Priority, &t.Tags, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scheduler_repo: get task: %w", err)
	}
	_ = json.Unmarshal(inputRaw, &t.InputJSON)
	return t, nil
}

func (r *schedulerRepository) ListTasks(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ScheduledTask], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM scheduled_tasks WHERE org_id = $1", orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("scheduler_repo: count: %w", err)
	}

	const sql = `
		SELECT id, org_id, name, description, task_type, agent_id, workflow_id,
		       input_prompt, input_json, provider_config_id, model_override,
		       schedule_type, cron_expression, interval_seconds, run_at,
		       status, last_run_at, next_run_at, run_count, max_runs,
		       retry_on_failure, max_retries, priority, tags, created_by, created_at, updated_at
		FROM scheduled_tasks WHERE org_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("scheduler_repo: list: %w", err)
	}
	defer rows.Close()

	var tasks []model.ScheduledTask
	for rows.Next() {
		var t model.ScheduledTask
		var inputRaw []byte
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.Name, &t.Description, &t.TaskType, &t.AgentID, &t.WorkflowID,
			&t.InputPrompt, &inputRaw, &t.ProviderConfigID, &t.ModelOverride,
			&t.ScheduleType, &t.CronExpression, &t.IntervalSeconds, &t.RunAt,
			&t.Status, &t.LastRunAt, &t.NextRunAt, &t.RunCount, &t.MaxRuns,
			&t.RetryOnFailure, &t.MaxRetries, &t.Priority, &t.Tags, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scheduler_repo: scan: %w", err)
		}
		_ = json.Unmarshal(inputRaw, &t.InputJSON)
		tasks = append(tasks, t)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.ScheduledTask]{
		Data: tasks, Total: total, Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, rows.Err()
}

func (r *schedulerRepository) UpdateTask(ctx context.Context, id uuid.UUID, req model.UpdateScheduledTaskRequest) (*model.ScheduledTask, error) {
	const sql = `
		UPDATE scheduled_tasks SET
		    name = COALESCE($2, name),
		    description = COALESCE($3, description),
		    input_prompt = COALESCE($4, input_prompt),
		    cron_expression = COALESCE($5, cron_expression),
		    interval_seconds = COALESCE($6, interval_seconds),
		    status = COALESCE($7, status),
		    max_runs = COALESCE($8, max_runs),
		    priority = COALESCE($9, priority),
		    model_override = COALESCE($10, model_override),
		    updated_at = NOW()
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, sql,
		id, req.Name, req.Description, req.InputPrompt,
		req.CronExpression, req.IntervalSeconds, req.Status,
		req.MaxRuns, req.Priority, req.ModelOverride,
	)
	if err != nil {
		return nil, fmt.Errorf("scheduler_repo: update: %w", err)
	}
	return r.GetTask(ctx, id)
}

func (r *schedulerRepository) DeleteTask(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, "DELETE FROM scheduled_tasks WHERE id = $1", id)
	return err
}

func (r *schedulerRepository) ListDueTasks(ctx context.Context) ([]model.ScheduledTask, error) {
	const sql = `
		SELECT id, org_id, name, description, task_type, agent_id, workflow_id,
		       input_prompt, input_json, provider_config_id, model_override,
		       schedule_type, cron_expression, interval_seconds, run_at,
		       status, last_run_at, next_run_at, run_count, max_runs,
		       retry_on_failure, max_retries, priority, tags, created_by, created_at, updated_at
		FROM scheduled_tasks
		WHERE status = 'active' AND next_run_at <= NOW()
		ORDER BY priority DESC, next_run_at`

	rows, err := r.pool.Query(ctx, sql)
	if err != nil {
		return nil, fmt.Errorf("scheduler_repo: list due: %w", err)
	}
	defer rows.Close()

	var tasks []model.ScheduledTask
	for rows.Next() {
		var t model.ScheduledTask
		var inputRaw []byte
		if err := rows.Scan(
			&t.ID, &t.OrgID, &t.Name, &t.Description, &t.TaskType, &t.AgentID, &t.WorkflowID,
			&t.InputPrompt, &inputRaw, &t.ProviderConfigID, &t.ModelOverride,
			&t.ScheduleType, &t.CronExpression, &t.IntervalSeconds, &t.RunAt,
			&t.Status, &t.LastRunAt, &t.NextRunAt, &t.RunCount, &t.MaxRuns,
			&t.RetryOnFailure, &t.MaxRetries, &t.Priority, &t.Tags, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scheduler_repo: scan due: %w", err)
		}
		_ = json.Unmarshal(inputRaw, &t.InputJSON)
		tasks = append(tasks, t)
	}
	return tasks, rows.Err()
}

func (r *schedulerRepository) IncrementRunCount(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE scheduled_tasks SET run_count = run_count + 1, last_run_at = NOW(), updated_at = NOW() WHERE id = $1", id)
	return err
}

func (r *schedulerRepository) CreateRun(ctx context.Context, run *model.ScheduledTaskRun) error {
	const sql = `
		INSERT INTO scheduled_task_runs
		    (id, scheduled_task_id, execution_id, workflow_run_id, status, output, error_message, started_at, completed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`

	_, err := r.pool.Exec(ctx, sql,
		run.ID, run.ScheduledTaskID, run.ExecutionID, run.WorkflowRunID,
		run.Status, run.Output, run.ErrorMessage, run.StartedAt, run.CompletedAt,
	)
	return err
}

func (r *schedulerRepository) ListRuns(ctx context.Context, taskID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ScheduledTaskRun], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM scheduled_task_runs WHERE scheduled_task_id = $1", taskID).Scan(&total); err != nil {
		return nil, err
	}

	const sql = `
		SELECT id, scheduled_task_id, execution_id, workflow_run_id, status,
		       output, error_message, started_at, completed_at, created_at
		FROM scheduled_task_runs WHERE scheduled_task_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, taskID, params.PerPage, params.Offset())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []model.ScheduledTaskRun
	for rows.Next() {
		var run model.ScheduledTaskRun
		if err := rows.Scan(
			&run.ID, &run.ScheduledTaskID, &run.ExecutionID, &run.WorkflowRunID,
			&run.Status, &run.Output, &run.ErrorMessage, &run.StartedAt, &run.CompletedAt, &run.CreatedAt,
		); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.ScheduledTaskRun]{
		Data: runs, Total: total, Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, rows.Err()
}
