package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/executor"
	"github.com/jobshout/server/internal/model"
)

// ExecutionRepository handles persistence for agent executions and tool calls.
type ExecutionRepository interface {
	Create(ctx context.Context, exec *model.AgentExecution) error
	MarkStarted(ctx context.Context, id uuid.UUID) error
	MarkCompleted(ctx context.Context, id uuid.UUID, output string, totalTokens int, iterations int) error
	MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, totalTokens int, iterations int) error
	RecordToolCall(ctx context.Context, call *model.ExecutionToolCall) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.AgentExecution, error)
	ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error)
	// PersistResult is a convenience method used by the workflow DAG engine.
	PersistResult(ctx context.Context, execID uuid.UUID, res executor.Result) error
}

type executionRepository struct {
	pool *pgxpool.Pool
}

// NewExecutionRepository creates a new ExecutionRepository backed by pgxpool.
func NewExecutionRepository(pool *pgxpool.Pool) ExecutionRepository {
	return &executionRepository{pool: pool}
}

func (r *executionRepository) Create(ctx context.Context, exec *model.AgentExecution) error {
	const sql = `
		INSERT INTO agent_executions
		    (id, agent_id, org_id, workflow_run_id, step_id, input_prompt, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`

	_, err := r.pool.Exec(ctx, sql,
		exec.ID, exec.AgentID, exec.OrgID,
		exec.WorkflowRunID, exec.StepID,
		exec.InputPrompt, model.ExecutionStatusPending,
	)
	if err != nil {
		return fmt.Errorf("execution_repo: create: %w", err)
	}
	return nil
}

func (r *executionRepository) MarkStarted(ctx context.Context, id uuid.UUID) error {
	const sql = `UPDATE agent_executions SET status = $2, started_at = $3 WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, model.ExecutionStatusRunning, time.Now())
	if err != nil {
		return fmt.Errorf("execution_repo: mark started: %w", err)
	}
	return nil
}

func (r *executionRepository) MarkCompleted(ctx context.Context, id uuid.UUID, output string, totalTokens int, iterations int) error {
	const sql = `
		UPDATE agent_executions
		SET status = $2, output = $3, total_tokens = $4, iterations = $5, completed_at = $6
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, model.ExecutionStatusCompleted, output, totalTokens, iterations, time.Now())
	if err != nil {
		return fmt.Errorf("execution_repo: mark completed: %w", err)
	}
	return nil
}

func (r *executionRepository) MarkFailed(ctx context.Context, id uuid.UUID, errMsg string, totalTokens int, iterations int) error {
	const sql = `
		UPDATE agent_executions
		SET status = $2, error_message = $3, total_tokens = $4, iterations = $5, completed_at = $6
		WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, model.ExecutionStatusFailed, errMsg, totalTokens, iterations, time.Now())
	if err != nil {
		return fmt.Errorf("execution_repo: mark failed: %w", err)
	}
	return nil
}

func (r *executionRepository) RecordToolCall(ctx context.Context, call *model.ExecutionToolCall) error {
	inputJSON, err := json.Marshal(call.Input)
	if err != nil {
		inputJSON = []byte("{}")
	}
	const sql = `
		INSERT INTO execution_tool_calls
		    (id, execution_id, tool_name, input, output, error_message, duration_ms, called_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`

	_, err = r.pool.Exec(ctx, sql,
		call.ID, call.ExecutionID, call.ToolName,
		inputJSON, call.Output, call.ErrorMessage, call.DurationMs,
	)
	if err != nil {
		return fmt.Errorf("execution_repo: record tool call: %w", err)
	}
	return nil
}

func (r *executionRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.AgentExecution, error) {
	const sql = `
		SELECT id, agent_id, org_id, workflow_run_id, step_id, input_prompt, output,
		       status, error_message, total_tokens, iterations, started_at, completed_at, created_at
		FROM agent_executions WHERE id = $1`

	exec := &model.AgentExecution{}
	if err := r.pool.QueryRow(ctx, sql, id).Scan(
		&exec.ID, &exec.AgentID, &exec.OrgID, &exec.WorkflowRunID, &exec.StepID,
		&exec.InputPrompt, &exec.Output, &exec.Status, &exec.ErrorMessage,
		&exec.TotalTokens, &exec.Iterations, &exec.StartedAt, &exec.CompletedAt, &exec.CreatedAt,
	); err != nil {
		return nil, fmt.Errorf("execution_repo: get by id: %w", err)
	}

	calls, err := r.loadToolCalls(ctx, id)
	if err != nil {
		return nil, err
	}
	exec.ToolCalls = calls
	return exec, nil
}

func (r *executionRepository) loadToolCalls(ctx context.Context, executionID uuid.UUID) ([]model.ExecutionToolCall, error) {
	const sql = `
		SELECT id, execution_id, tool_name, input, output, error_message, duration_ms, called_at
		FROM execution_tool_calls WHERE execution_id = $1 ORDER BY called_at`

	rows, err := r.pool.Query(ctx, sql, executionID)
	if err != nil {
		return nil, fmt.Errorf("execution_repo: list tool calls: %w", err)
	}
	defer rows.Close()

	var calls []model.ExecutionToolCall
	for rows.Next() {
		var c model.ExecutionToolCall
		var inputRaw []byte
		if err := rows.Scan(
			&c.ID, &c.ExecutionID, &c.ToolName,
			&inputRaw, &c.Output, &c.ErrorMessage, &c.DurationMs, &c.CalledAt,
		); err != nil {
			return nil, fmt.Errorf("execution_repo: scan tool call: %w", err)
		}
		if err := json.Unmarshal(inputRaw, &c.Input); err != nil {
			c.Input = map[string]any{}
		}
		calls = append(calls, c)
	}
	return calls, rows.Err()
}

func (r *executionRepository) ListByAgent(ctx context.Context, agentID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.AgentExecution], error) {
	params.Normalize()

	const countSQL = `SELECT COUNT(*) FROM agent_executions WHERE agent_id = $1`
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, agentID).Scan(&total); err != nil {
		return nil, fmt.Errorf("execution_repo: count executions: %w", err)
	}

	const listSQL = `
		SELECT id, agent_id, org_id, workflow_run_id, step_id, input_prompt, output,
		       status, error_message, total_tokens, iterations, started_at, completed_at, created_at
		FROM agent_executions WHERE agent_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, listSQL, agentID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("execution_repo: list executions: %w", err)
	}
	defer rows.Close()

	var execs []model.AgentExecution
	for rows.Next() {
		var exec model.AgentExecution
		if err := rows.Scan(
			&exec.ID, &exec.AgentID, &exec.OrgID, &exec.WorkflowRunID, &exec.StepID,
			&exec.InputPrompt, &exec.Output, &exec.Status, &exec.ErrorMessage,
			&exec.TotalTokens, &exec.Iterations, &exec.StartedAt, &exec.CompletedAt, &exec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("execution_repo: scan execution: %w", err)
		}
		execs = append(execs, exec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("execution_repo: iterate executions: %w", err)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.AgentExecution]{
		Data: execs, Total: total,
		Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, nil
}

func (r *executionRepository) PersistResult(ctx context.Context, execID uuid.UUID, res executor.Result) error {
	if res.Err != nil {
		errMsg := res.Err.Error()
		if err := r.MarkFailed(ctx, execID, errMsg, res.TotalTokens, res.Iterations); err != nil {
			return err
		}
	} else {
		if err := r.MarkCompleted(ctx, execID, res.FinalAnswer, res.TotalTokens, res.Iterations); err != nil {
			return err
		}
	}

	// Persist tool call records.
	for _, tc := range res.ToolCalls {
		call := &model.ExecutionToolCall{
			ID:          uuid.New(),
			ExecutionID: execID,
			ToolName:    tc.ToolName,
			Input:       tc.Input,
			DurationMs:  tc.DurationMs,
		}
		if tc.Output != "" {
			call.Output = &tc.Output
		}
		if tc.Err != nil {
			errMsg := tc.Err.Error()
			call.ErrorMessage = &errMsg
		}
		// Non-fatal if tool call recording fails.
		_ = r.RecordToolCall(ctx, call)
	}
	return nil
}
