package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// WorkflowRepository handles database operations for workflows and their runs.
type WorkflowRepository interface {
	Create(ctx context.Context, wf *model.Workflow) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Workflow, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Workflow], error)
	Update(ctx context.Context, wf *model.Workflow) error
	Delete(ctx context.Context, id uuid.UUID) error

	CreateRun(ctx context.Context, run *model.WorkflowRun) error
	UpdateRun(ctx context.Context, run *model.WorkflowRun) error
	GetRunByID(ctx context.Context, id uuid.UUID) (*model.WorkflowRun, error)
	ListRuns(ctx context.Context, workflowID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.WorkflowRun], error)
}

type workflowRepository struct {
	pool *pgxpool.Pool
}

// NewWorkflowRepository creates a new WorkflowRepository backed by pgxpool.
func NewWorkflowRepository(pool *pgxpool.Pool) WorkflowRepository {
	return &workflowRepository{pool: pool}
}

func (r *workflowRepository) Create(ctx context.Context, wf *model.Workflow) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("workflow_repo: begin transaction: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	const wfSQL = `
		INSERT INTO workflows (id, org_id, name, description, status, created_by, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW(), NOW())`

	if _, err = tx.Exec(ctx, wfSQL,
		wf.ID, wf.OrgID, wf.Name, wf.Description, wf.Status, wf.CreatedBy,
	); err != nil {
		return fmt.Errorf("workflow_repo: insert workflow: %w", err)
	}

	for i := range wf.Steps {
		step := &wf.Steps[i]
		const stepSQL = `
			INSERT INTO workflow_steps (id, workflow_id, name, agent_id, input_template, position, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, NOW())`

		if _, err = tx.Exec(ctx, stepSQL,
			step.ID, wf.ID, step.Name, step.AgentID, step.InputTemplate, step.Position,
		); err != nil {
			return fmt.Errorf("workflow_repo: insert step %q: %w", step.Name, err)
		}

		for _, depName := range step.DependsOn {
			// Resolve dependency step ID by name within this workflow.
			var depID uuid.UUID
			const resolveSQL = `SELECT id FROM workflow_steps WHERE workflow_id = $1 AND name = $2`
			if err = tx.QueryRow(ctx, resolveSQL, wf.ID, depName).Scan(&depID); err != nil {
				return fmt.Errorf("workflow_repo: resolve dependency step %q: %w", depName, err)
			}
			const depSQL = `INSERT INTO workflow_step_deps (step_id, depends_on_id) VALUES ($1, $2)`
			if _, err = tx.Exec(ctx, depSQL, step.ID, depID); err != nil {
				return fmt.Errorf("workflow_repo: insert step dependency: %w", err)
			}
		}
	}

	return tx.Commit(ctx)
}

func (r *workflowRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Workflow, error) {
	const wfSQL = `
		SELECT id, org_id, name, description, status, created_by, created_at, updated_at
		FROM workflows WHERE id = $1`

	wf := &model.Workflow{}
	if err := r.pool.QueryRow(ctx, wfSQL, id).Scan(
		&wf.ID, &wf.OrgID, &wf.Name, &wf.Description, &wf.Status,
		&wf.CreatedBy, &wf.CreatedAt, &wf.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("workflow_repo: get workflow: %w", err)
	}

	steps, err := r.loadSteps(ctx, id)
	if err != nil {
		return nil, err
	}
	wf.Steps = steps
	return wf, nil
}

func (r *workflowRepository) loadSteps(ctx context.Context, workflowID uuid.UUID) ([]model.WorkflowStep, error) {
	const stepsSQL = `
		SELECT id, workflow_id, name, agent_id, input_template, position, created_at
		FROM workflow_steps WHERE workflow_id = $1 ORDER BY position`

	rows, err := r.pool.Query(ctx, stepsSQL, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow_repo: list steps: %w", err)
	}
	defer rows.Close()

	var steps []model.WorkflowStep
	for rows.Next() {
		var s model.WorkflowStep
		if err := rows.Scan(&s.ID, &s.WorkflowID, &s.Name, &s.AgentID, &s.InputTemplate, &s.Position, &s.CreatedAt); err != nil {
			return nil, fmt.Errorf("workflow_repo: scan step: %w", err)
		}
		steps = append(steps, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workflow_repo: iterate steps: %w", err)
	}

	// Load dependencies for each step.
	for i := range steps {
		deps, err := r.loadStepDeps(ctx, steps[i].ID, workflowID)
		if err != nil {
			return nil, err
		}
		steps[i].DependsOn = deps
	}
	return steps, nil
}

func (r *workflowRepository) loadStepDeps(ctx context.Context, stepID uuid.UUID, workflowID uuid.UUID) ([]string, error) {
	const sql = `
		SELECT ws.name
		FROM workflow_step_deps d
		JOIN workflow_steps ws ON ws.id = d.depends_on_id
		WHERE d.step_id = $1 AND ws.workflow_id = $2`

	rows, err := r.pool.Query(ctx, sql, stepID, workflowID)
	if err != nil {
		return nil, fmt.Errorf("workflow_repo: load step deps: %w", err)
	}
	defer rows.Close()

	var deps []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, fmt.Errorf("workflow_repo: scan dep name: %w", err)
		}
		deps = append(deps, name)
	}
	return deps, rows.Err()
}

func (r *workflowRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Workflow], error) {
	params.Normalize()

	const countSQL = `SELECT COUNT(*) FROM workflows WHERE org_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("workflow_repo: count workflows: %w", err)
	}

	const listSQL = `
		SELECT id, org_id, name, description, status, created_by, created_at, updated_at
		FROM workflows WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, listSQL, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("workflow_repo: list workflows: %w", err)
	}
	defer rows.Close()

	var wfs []model.Workflow
	for rows.Next() {
		var wf model.Workflow
		if err := rows.Scan(&wf.ID, &wf.OrgID, &wf.Name, &wf.Description, &wf.Status,
			&wf.CreatedBy, &wf.CreatedAt, &wf.UpdatedAt); err != nil {
			return nil, fmt.Errorf("workflow_repo: scan workflow: %w", err)
		}
		wfs = append(wfs, wf)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workflow_repo: iterate workflows: %w", err)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.Workflow]{
		Data:       wfs,
		Total:      total,
		Page:       params.Page,
		PerPage:    params.PerPage,
		TotalPages: totalPages,
	}, nil
}

func (r *workflowRepository) Update(ctx context.Context, wf *model.Workflow) error {
	const sql = `
		UPDATE workflows SET name = $2, description = $3, status = $4, updated_at = NOW()
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, wf.ID, wf.Name, wf.Description, wf.Status)
	if err != nil {
		return fmt.Errorf("workflow_repo: update workflow: %w", err)
	}
	return nil
}

func (r *workflowRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM workflows WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("workflow_repo: delete workflow: %w", err)
	}
	return nil
}

// --- Workflow Runs ---

func (r *workflowRepository) CreateRun(ctx context.Context, run *model.WorkflowRun) error {
	inputJSON, err := json.Marshal(run.Input)
	if err != nil {
		return fmt.Errorf("workflow_repo: marshal run input: %w", err)
	}
	const sql = `
		INSERT INTO workflow_runs (id, workflow_id, org_id, status, input, triggered_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())`
	_, err = r.pool.Exec(ctx, sql,
		run.ID, run.WorkflowID, run.OrgID, run.Status, inputJSON, run.TriggeredBy,
	)
	if err != nil {
		return fmt.Errorf("workflow_repo: create run: %w", err)
	}
	return nil
}

func (r *workflowRepository) UpdateRun(ctx context.Context, run *model.WorkflowRun) error {
	outputsJSON, err := json.Marshal(run.Outputs)
	if err != nil {
		return fmt.Errorf("workflow_repo: marshal run outputs: %w", err)
	}
	const sql = `
		UPDATE workflow_runs
		SET status = $2, outputs = $3, error_message = $4, started_at = $5, completed_at = $6
		WHERE id = $1`
	_, err = r.pool.Exec(ctx, sql,
		run.ID, run.Status, outputsJSON, run.ErrorMessage, run.StartedAt, run.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("workflow_repo: update run: %w", err)
	}
	return nil
}

func (r *workflowRepository) GetRunByID(ctx context.Context, id uuid.UUID) (*model.WorkflowRun, error) {
	const sql = `
		SELECT id, workflow_id, org_id, status, input, outputs, error_message,
		       started_at, completed_at, triggered_by, created_at
		FROM workflow_runs WHERE id = $1`

	run := &model.WorkflowRun{}
	var inputRaw, outputsRaw []byte
	if err := r.pool.QueryRow(ctx, sql, id).Scan(
		&run.ID, &run.WorkflowID, &run.OrgID, &run.Status,
		&inputRaw, &outputsRaw, &run.ErrorMessage,
		&run.StartedAt, &run.CompletedAt, &run.TriggeredBy, &run.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("workflow_repo: get run: %w", err)
	}

	if err := json.Unmarshal(inputRaw, &run.Input); err != nil {
		run.Input = map[string]any{}
	}
	if err := json.Unmarshal(outputsRaw, &run.Outputs); err != nil {
		run.Outputs = map[string]string{}
	}
	return run, nil
}

func (r *workflowRepository) ListRuns(ctx context.Context, workflowID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.WorkflowRun], error) {
	params.Normalize()

	const countSQL = `SELECT COUNT(*) FROM workflow_runs WHERE workflow_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, workflowID).Scan(&total); err != nil {
		return nil, fmt.Errorf("workflow_repo: count runs: %w", err)
	}

	const listSQL = `
		SELECT id, workflow_id, org_id, status, input, outputs, error_message,
		       started_at, completed_at, triggered_by, created_at
		FROM workflow_runs WHERE workflow_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, listSQL, workflowID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("workflow_repo: list runs: %w", err)
	}
	defer rows.Close()

	var runs []model.WorkflowRun
	for rows.Next() {
		var run model.WorkflowRun
		var inputRaw, outputsRaw []byte
		if err := rows.Scan(
			&run.ID, &run.WorkflowID, &run.OrgID, &run.Status,
			&inputRaw, &outputsRaw, &run.ErrorMessage,
			&run.StartedAt, &run.CompletedAt, &run.TriggeredBy, &run.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("workflow_repo: scan run: %w", err)
		}
		if err := json.Unmarshal(inputRaw, &run.Input); err != nil {
			run.Input = map[string]any{}
		}
		if err := json.Unmarshal(outputsRaw, &run.Outputs); err != nil {
			run.Outputs = map[string]string{}
		}
		runs = append(runs, run)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workflow_repo: iterate runs: %w", err)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.WorkflowRun]{
		Data: runs, Total: total,
		Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, nil
}

