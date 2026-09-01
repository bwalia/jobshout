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

type ReviewRunRepository interface {
	Create(ctx context.Context, run *model.ReviewRun) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.ReviewRun, error)
	Update(ctx context.Context, run *model.ReviewRun) error
	ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.ReviewRun], error)
	FindActive(ctx context.Context, orgID uuid.UUID, repo string, prNumber int) (*model.ReviewRun, error)
	ClaimDueRuns(ctx context.Context, limit int, lease time.Duration) ([]model.ReviewRun, error)
}

type reviewRunRepository struct {
	pool *pgxpool.Pool
}

func NewReviewRunRepository(pool *pgxpool.Pool) ReviewRunRepository {
	return &reviewRunRepository{pool: pool}
}

const reviewRunColumns = `
	id, org_id, agent_id, requested_by, repo, pr_number, dry_run, force, status,
	remote_job_id, head_sha, decision, verdict, summary, github_url, result, stage_log,
	error_message, poll_attempts, next_poll_at, started_at, completed_at, created_at, updated_at, task_id`

func scanReviewRun(row pgx.Row) (*model.ReviewRun, error) {
	run := &model.ReviewRun{}
	var result, stageLog []byte
	if err := row.Scan(
		&run.ID, &run.OrgID, &run.AgentID, &run.RequestedBy, &run.Repo, &run.PRNumber,
		&run.DryRun, &run.Force, &run.Status, &run.RemoteJobID, &run.HeadSHA, &run.Decision,
		&run.Verdict, &run.Summary, &run.GitHubURL, &result, &stageLog, &run.ErrorMessage,
		&run.PollAttempts, &run.NextPollAt, &run.StartedAt, &run.CompletedAt,
		&run.CreatedAt, &run.UpdatedAt, &run.TaskID,
	); err != nil {
		return nil, err
	}
	if len(result) > 0 {
		run.Result = json.RawMessage(result)
	}
	if len(stageLog) > 0 {
		_ = json.Unmarshal(stageLog, &run.StageLog)
	}
	return run, nil
}

func stageLogJSON(log []string) []byte {
	if log == nil {
		log = []string{}
	}
	b, _ := json.Marshal(log)
	return b
}

func (r *reviewRunRepository) Create(ctx context.Context, run *model.ReviewRun) error {
	query := `
		INSERT INTO review_runs (
			id, org_id, agent_id, requested_by, repo, pr_number, dry_run, force, status,
			remote_job_id, error_message, poll_attempts, next_poll_at, stage_log,
			task_id, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9,
			$10, $11, $12, $13, $14, $15, NOW(), NOW()
		)
		RETURNING created_at, updated_at`
	return r.pool.QueryRow(ctx, query,
		run.ID, run.OrgID, run.AgentID, run.RequestedBy, run.Repo, run.PRNumber,
		run.DryRun, run.Force, run.Status, run.RemoteJobID, run.ErrorMessage,
		run.PollAttempts, run.NextPollAt, stageLogJSON(run.StageLog), run.TaskID,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *reviewRunRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.ReviewRun, error) {
	run, err := scanReviewRun(r.pool.QueryRow(ctx, `SELECT `+reviewRunColumns+` FROM review_runs WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("review run not found")
		}
		return nil, fmt.Errorf("finding review run: %w", err)
	}
	return run, nil
}

func (r *reviewRunRepository) Update(ctx context.Context, run *model.ReviewRun) error {
	query := `
		UPDATE review_runs
		SET status = $2, remote_job_id = $3, head_sha = $4, decision = $5, verdict = $6,
		    summary = $7, github_url = $8, result = $9, stage_log = $10, error_message = $11,
		    poll_attempts = $12, next_poll_at = $13, started_at = $14, completed_at = $15,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING created_at, updated_at`
	var result any
	if len(run.Result) > 0 {
		result = []byte(run.Result)
	}
	return r.pool.QueryRow(ctx, query,
		run.ID, run.Status, run.RemoteJobID, run.HeadSHA, run.Decision, run.Verdict,
		run.Summary, run.GitHubURL, result, stageLogJSON(run.StageLog), run.ErrorMessage,
		run.PollAttempts, run.NextPollAt, run.StartedAt, run.CompletedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *reviewRunRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.ReviewRun], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM review_runs WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("counting review runs: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+reviewRunColumns+`
		FROM review_runs WHERE org_id = $1
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`, orgID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, fmt.Errorf("listing review runs: %w", err)
	}
	defer rows.Close()
	runs := make([]model.ReviewRun, 0, pagination.PerPage)
	for rows.Next() {
		run, err := scanReviewRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning review run: %w", err)
		}
		runs = append(runs, *run)
	}
	return &model.PaginatedResponse[model.ReviewRun]{
		Data:       runs,
		Total:      total,
		Page:       pagination.Page,
		PerPage:    pagination.PerPage,
		TotalPages: (total + pagination.PerPage - 1) / pagination.PerPage,
	}, nil
}

func (r *reviewRunRepository) FindActive(ctx context.Context, orgID uuid.UUID, repo string, prNumber int) (*model.ReviewRun, error) {
	run, err := scanReviewRun(r.pool.QueryRow(ctx, `
		SELECT `+reviewRunColumns+`
		FROM review_runs
		WHERE org_id = $1 AND repo = $2 AND pr_number = $3
		  AND status IN ('queued', 'running')
		ORDER BY created_at DESC
		LIMIT 1`, orgID, repo, prNumber))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("finding active review run: %w", err)
	}
	return run, nil
}

func (r *reviewRunRepository) ClaimDueRuns(ctx context.Context, limit int, lease time.Duration) ([]model.ReviewRun, error) {
	query := `
		UPDATE review_runs
		SET next_poll_at = NOW() + $2::interval, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM review_runs
			WHERE status IN ('queued', 'running')
			  AND (next_poll_at IS NULL OR next_poll_at <= NOW())
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING ` + reviewRunColumns
	leaseInterval := fmt.Sprintf("%d milliseconds", lease.Milliseconds())
	rows, err := r.pool.Query(ctx, query, limit, leaseInterval)
	if err != nil {
		return nil, fmt.Errorf("claiming due review runs: %w", err)
	}
	defer rows.Close()
	runs := make([]model.ReviewRun, 0, limit)
	for rows.Next() {
		run, err := scanReviewRun(rows)
		if err != nil {
			return nil, fmt.Errorf("scanning claimed review run: %w", err)
		}
		runs = append(runs, *run)
	}
	return runs, rows.Err()
}
