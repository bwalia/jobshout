package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// WAFLabRunRepository persists WAF Efficacy Lab runs, steps, and results.
type WAFLabRunRepository interface {
	Create(ctx context.Context, run *model.WAFLabRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.WAFLabRun, error)
	Update(ctx context.Context, run *model.WAFLabRun) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.WAFLabRun], error)
	CreateStep(ctx context.Context, step *model.WAFLabStep) error
	ListSteps(ctx context.Context, runID uuid.UUID) ([]model.WAFLabStep, error)
	CreateResult(ctx context.Context, result *model.WAFLabResult) error
	ListResults(ctx context.Context, runID uuid.UUID) ([]model.WAFLabResult, error)
	// Finalize writes terminal run state plus steps and results in one transaction.
	Finalize(ctx context.Context, run *model.WAFLabRun, steps []model.WAFLabStep, results []model.WAFLabResult, score *model.WAFLabScore) error
}

type wafLabRunRepository struct {
	pool *pgxpool.Pool
}

// NewWAFLabRunRepository constructs a Postgres-backed repository.
func NewWAFLabRunRepository(pool *pgxpool.Pool) WAFLabRunRepository {
	return &wafLabRunRepository{pool: pool}
}

const wafLabRunColumns = `
	id, agent_id, task_id, org_id, status, mode, secure_host, open_host,
	origin_upstream, policy_id, manage_dns, dns_zone, attack_set, instruction,
	wslproxy_base_url, score, error_message, requested_by, started_at, completed_at,
	created_at, updated_at`

func scanWAFLabRun(row pgx.Row) (*model.WAFLabRun, error) {
	run := &model.WAFLabRun{}
	var scoreJSON []byte
	if err := row.Scan(
		&run.ID, &run.AgentID, &run.TaskID, &run.OrgID, &run.Status, &run.Mode,
		&run.SecureHost, &run.OpenHost, &run.OriginUpstream, &run.PolicyID,
		&run.ManageDNS, &run.DNSZone, &run.AttackSet, &run.Instruction,
		&run.WSLProxyBaseURL, &scoreJSON, &run.ErrorMessage, &run.RequestedBy,
		&run.StartedAt, &run.CompletedAt, &run.CreatedAt, &run.UpdatedAt,
	); err != nil {
		return nil, err
	}
	if len(scoreJSON) > 0 {
		var score model.WAFLabScore
		if err := json.Unmarshal(scoreJSON, &score); err == nil {
			run.Score = &score
		}
	}
	return run, nil
}

func (r *wafLabRunRepository) Create(ctx context.Context, run *model.WAFLabRun) error {
	scoreJSON, err := marshalScore(run.Score)
	if err != nil {
		return err
	}
	query := `
		INSERT INTO waf_lab_runs (
			id, agent_id, task_id, org_id, status, mode, secure_host, open_host,
			origin_upstream, policy_id, manage_dns, dns_zone, attack_set, instruction,
			wslproxy_base_url, score, error_message, requested_by, started_at, completed_at,
			created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
			NOW(), NOW()
		)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		run.ID, run.AgentID, run.TaskID, run.OrgID, run.Status, run.Mode,
		run.SecureHost, run.OpenHost, run.OriginUpstream, run.PolicyID,
		run.ManageDNS, run.DNSZone, run.AttackSet, run.Instruction,
		run.WSLProxyBaseURL, scoreJSON, run.ErrorMessage, run.RequestedBy,
		run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *wafLabRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.WAFLabRun, error) {
	query := `SELECT ` + wafLabRunColumns + ` FROM waf_lab_runs WHERE id = $1`
	run, err := scanWAFLabRun(r.pool.QueryRow(ctx, query, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("waf lab run not found")
		}
		return nil, fmt.Errorf("finding waf lab run: %w", err)
	}
	return run, nil
}

func (r *wafLabRunRepository) Update(ctx context.Context, run *model.WAFLabRun) error {
	scoreJSON, err := marshalScore(run.Score)
	if err != nil {
		return err
	}
	query := `
		UPDATE waf_lab_runs
		SET status = $2, mode = $3, secure_host = $4, open_host = $5,
		    origin_upstream = $6, policy_id = $7, manage_dns = $8, dns_zone = $9,
		    attack_set = $10, instruction = $11, wslproxy_base_url = $12, score = $13,
		    error_message = $14, requested_by = $15, started_at = $16, completed_at = $17,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		run.ID, run.Status, run.Mode, run.SecureHost, run.OpenHost,
		run.OriginUpstream, run.PolicyID, run.ManageDNS, run.DNSZone,
		run.AttackSet, run.Instruction, run.WSLProxyBaseURL, scoreJSON,
		run.ErrorMessage, run.RequestedBy, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *wafLabRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.WAFLabRun], error) {
	pagination.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM waf_lab_runs WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting waf lab runs: %w", err)
	}

	query := `
		SELECT ` + wafLabRunColumns + `
		FROM waf_lab_runs
		WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`
	rows, err := r.pool.Query(ctx, query, orgID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, fmt.Errorf("listing waf lab runs: %w", err)
	}
	defer rows.Close()

	runs := make([]model.WAFLabRun, 0, pagination.PerPage)
	for rows.Next() {
		run, err := scanWAFLabRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning waf lab run: %w", err)
		}
		runs = append(runs, *run)
	}

	return &model.PaginatedResponse[model.WAFLabRun]{
		Data:       runs,
		Total:      total,
		Page:       pagination.Page,
		PerPage:    pagination.PerPage,
		TotalPages: (total + pagination.PerPage - 1) / pagination.PerPage,
	}, nil
}

func (r *wafLabRunRepository) CreateStep(ctx context.Context, step *model.WAFLabStep) error {
	if step.ID == uuid.Nil {
		step.ID = uuid.New()
	}
	query := `
		INSERT INTO waf_lab_steps (id, run_id, phase, status, message, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		step.ID, step.RunID, step.Phase, step.Status, step.Message, step.StartedAt, step.EndedAt,
	).Scan(&step.CreatedAt)
}

func (r *wafLabRunRepository) ListSteps(ctx context.Context, runID uuid.UUID) ([]model.WAFLabStep, error) {
	query := `
		SELECT id, run_id, phase, status, message, started_at, ended_at, created_at
		FROM waf_lab_steps
		WHERE run_id = $1
		ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("listing waf lab steps: %w", err)
	}
	defer rows.Close()

	steps := make([]model.WAFLabStep, 0)
	for rows.Next() {
		var s model.WAFLabStep
		if err := rows.Scan(
			&s.ID, &s.RunID, &s.Phase, &s.Status, &s.Message, &s.StartedAt, &s.EndedAt, &s.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning waf lab step: %w", err)
		}
		steps = append(steps, s)
	}
	return steps, nil
}

func (r *wafLabRunRepository) CreateResult(ctx context.Context, result *model.WAFLabResult) error {
	if result.ID == uuid.Nil {
		result.ID = uuid.New()
	}
	query := `
		INSERT INTO waf_lab_results (
			id, run_id, host_role, host, attack_id, attack_name, category, method, path,
			payload, status_code, blocked, expect_block, waf_rule, waf_violation, support_id,
			latency_ms, verdict, notes, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW()
		)
		RETURNING created_at`
	return r.pool.QueryRow(ctx, query,
		result.ID, result.RunID, result.HostRole, result.Host, result.AttackID, result.AttackName,
		result.Category, result.Method, result.Path, result.Payload, result.StatusCode,
		result.Blocked, result.ExpectBlock, result.WAFRule, result.WAFViolation, result.SupportID,
		result.LatencyMS, result.Verdict, result.Notes,
	).Scan(&result.CreatedAt)
}

func (r *wafLabRunRepository) ListResults(ctx context.Context, runID uuid.UUID) ([]model.WAFLabResult, error) {
	query := `
		SELECT id, run_id, host_role, host, attack_id, attack_name, category, method, path,
		       payload, status_code, blocked, expect_block, waf_rule, waf_violation, support_id,
		       latency_ms, verdict, notes, created_at
		FROM waf_lab_results
		WHERE run_id = $1
		ORDER BY created_at ASC`
	rows, err := r.pool.Query(ctx, query, runID)
	if err != nil {
		return nil, fmt.Errorf("listing waf lab results: %w", err)
	}
	defer rows.Close()

	out := make([]model.WAFLabResult, 0)
	for rows.Next() {
		var res model.WAFLabResult
		if err := rows.Scan(
			&res.ID, &res.RunID, &res.HostRole, &res.Host, &res.AttackID, &res.AttackName,
			&res.Category, &res.Method, &res.Path, &res.Payload, &res.StatusCode,
			&res.Blocked, &res.ExpectBlock, &res.WAFRule, &res.WAFViolation, &res.SupportID,
			&res.LatencyMS, &res.Verdict, &res.Notes, &res.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning waf lab result: %w", err)
		}
		out = append(out, res)
	}
	return out, nil
}

func (r *wafLabRunRepository) Finalize(ctx context.Context, run *model.WAFLabRun, steps []model.WAFLabStep, results []model.WAFLabResult, score *model.WAFLabScore) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("waf_lab_repo: begin finalize: %w", err)
	}
	defer tx.Rollback(ctx) //nolint:errcheck

	run.Score = score
	scoreJSON, err := marshalScore(score)
	if err != nil {
		return err
	}

	const runSQL = `
		UPDATE waf_lab_runs
		SET status = $2, score = $3, error_message = $4, started_at = $5, completed_at = $6,
		    updated_at = NOW()
		WHERE id = $1`
	if _, err := tx.Exec(ctx, runSQL,
		run.ID, run.Status, scoreJSON, run.ErrorMessage, run.StartedAt, run.CompletedAt,
	); err != nil {
		return fmt.Errorf("waf_lab_repo: finalize run: %w", err)
	}

	const stepSQL = `
		INSERT INTO waf_lab_steps (id, run_id, phase, status, message, started_at, ended_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())`
	now := time.Now()
	for i := range steps {
		s := &steps[i]
		if s.ID == uuid.Nil {
			s.ID = uuid.New()
		}
		s.RunID = run.ID
		if s.EndedAt == nil && (s.Status == "completed" || s.Status == "failed" || s.Status == "skipped") {
			ended := now
			s.EndedAt = &ended
		}
		if _, err := tx.Exec(ctx, stepSQL,
			s.ID, s.RunID, s.Phase, s.Status, s.Message, s.StartedAt, s.EndedAt,
		); err != nil {
			return fmt.Errorf("waf_lab_repo: insert step: %w", err)
		}
	}

	const resultSQL = `
		INSERT INTO waf_lab_results (
			id, run_id, host_role, host, attack_id, attack_name, category, method, path,
			payload, status_code, blocked, expect_block, waf_rule, waf_violation, support_id,
			latency_ms, verdict, notes, created_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, NOW()
		)`
	for i := range results {
		res := &results[i]
		if res.ID == uuid.Nil {
			res.ID = uuid.New()
		}
		res.RunID = run.ID
		if _, err := tx.Exec(ctx, resultSQL,
			res.ID, res.RunID, res.HostRole, res.Host, res.AttackID, res.AttackName,
			res.Category, res.Method, res.Path, res.Payload, res.StatusCode,
			res.Blocked, res.ExpectBlock, res.WAFRule, res.WAFViolation, res.SupportID,
			res.LatencyMS, res.Verdict, res.Notes,
		); err != nil {
			return fmt.Errorf("waf_lab_repo: insert result: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func marshalScore(score *model.WAFLabScore) ([]byte, error) {
	if score == nil {
		return nil, nil
	}
	b, err := json.Marshal(score)
	if err != nil {
		return nil, fmt.Errorf("marshal waf lab score: %w", err)
	}
	return b, nil
}
