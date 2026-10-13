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

// ErrVideoRunNotFound is returned for a run that does not exist in the
// caller's org.
var ErrVideoRunNotFound = errors.New("video: run not found")

// VideoRunRepository stores Video Producer runs.
type VideoRunRepository interface {
	Create(ctx context.Context, run *model.VideoRun) error
	// Save writes every field that changes as a run moves.
	Save(ctx context.Context, run *model.VideoRun) error
	Get(ctx context.Context, orgID, id uuid.UUID) (*model.VideoRun, error)
	List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.VideoRun, error)
	// Rendering lists runs handed to the workstation and not yet finished, in
	// every org: what a restarted server must go back to following.
	Rendering(ctx context.Context) ([]model.VideoRun, error)
	// UnfinishedForTask lists a task's runs that are still going.
	UnfinishedForTask(ctx context.Context, taskID uuid.UUID) ([]model.VideoRun, error)
	// FailStaleScripting fails runs still writing their script since before
	// cutoff: their writer died with its server. Returns how many.
	FailStaleScripting(ctx context.Context, cutoff time.Time, msg string) (int, error)
}

type videoRunRepository struct {
	pool *pgxpool.Pool
}

// NewVideoRunRepository constructs the repository.
func NewVideoRunRepository(pool *pgxpool.Pool) VideoRunRepository {
	return &videoRunRepository{pool: pool}
}

const videoRunCols = `id, org_id, task_id, agent_id, requested_by, topic, brief, title, shots, status, stage,
	progress, message, error_message, notes, video_url, duration_seconds, motion_model, estimate_minutes,
	created_at, started_at, finished_at, updated_at`

func scanVideoRun(row pgx.Row) (*model.VideoRun, error) {
	var (
		r                   model.VideoRun
		brief, shots, notes []byte
	)
	if err := row.Scan(&r.ID, &r.OrgID, &r.TaskID, &r.AgentID, &r.RequestedBy, &r.Topic, &brief, &r.Title,
		&shots, &r.Status, &r.Stage, &r.Progress, &r.Message, &r.Error, &notes, &r.VideoURL,
		&r.DurationSeconds, &r.MotionModel, &r.EstimateMinutes, &r.CreatedAt, &r.StartedAt, &r.FinishedAt,
		&r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrVideoRunNotFound
		}
		return nil, fmt.Errorf("video_repo: scan: %w", err)
	}
	_ = json.Unmarshal(brief, &r.Brief)
	_ = json.Unmarshal(shots, &r.Shots)
	_ = json.Unmarshal(notes, &r.Notes)
	if r.Shots == nil {
		r.Shots = []model.VideoShot{}
	}
	if r.Notes == nil {
		r.Notes = []string{}
	}
	return &r, nil
}

func (r *videoRunRepository) Create(ctx context.Context, run *model.VideoRun) error {
	brief, err := json.Marshal(run.Brief)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO video_runs (id, org_id, task_id, agent_id, requested_by, topic, brief, status, stage,
			message, estimate_minutes, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,NOW(),NOW())
		RETURNING created_at, updated_at`,
		run.ID, run.OrgID, run.TaskID, run.AgentID, run.RequestedBy, run.Topic, brief, run.Status, run.Stage,
		run.Message, run.EstimateMinutes,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *videoRunRepository) Save(ctx context.Context, run *model.VideoRun) error {
	shots, err := json.Marshal(emptyIfNil(run.Shots))
	if err != nil {
		return err
	}
	notes, err := json.Marshal(emptyIfNil(run.Notes))
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE video_runs SET title = $2, shots = $3, status = $4, stage = $5, progress = $6, message = $7,
			error_message = $8, notes = $9, video_url = $10, duration_seconds = $11, motion_model = $12,
			estimate_minutes = $13, started_at = $14, finished_at = $15, updated_at = NOW()
		WHERE id = $1`,
		run.ID, run.Title, shots, run.Status, run.Stage, run.Progress, run.Message, run.Error, notes,
		run.VideoURL, run.DurationSeconds, run.MotionModel, run.EstimateMinutes, run.StartedAt, run.FinishedAt)
	if err != nil {
		return fmt.Errorf("video_repo: save: %w", err)
	}
	return nil
}

func emptyIfNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

func (r *videoRunRepository) Get(ctx context.Context, orgID, id uuid.UUID) (*model.VideoRun, error) {
	return scanVideoRun(r.pool.QueryRow(ctx, `SELECT `+videoRunCols+` FROM video_runs WHERE org_id = $1 AND id = $2`,
		orgID, id))
}

func (r *videoRunRepository) list(ctx context.Context, query string, args ...any) ([]model.VideoRun, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+videoRunCols+` FROM video_runs `+query, args...)
	if err != nil {
		return nil, fmt.Errorf("video_repo: list: %w", err)
	}
	defer rows.Close()
	out := []model.VideoRun{}
	for rows.Next() {
		run, err := scanVideoRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func (r *videoRunRepository) List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.VideoRun, error) {
	return r.list(ctx, `WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
}

func (r *videoRunRepository) Rendering(ctx context.Context) ([]model.VideoRun, error) {
	return r.list(ctx, `WHERE status IN ('queued', 'rendering') ORDER BY created_at`)
}

func (r *videoRunRepository) UnfinishedForTask(ctx context.Context, taskID uuid.UUID) ([]model.VideoRun, error) {
	return r.list(ctx, `WHERE task_id = $1 AND status IN ('scripting', 'queued', 'rendering') ORDER BY created_at`, taskID)
}

func (r *videoRunRepository) FailStaleScripting(ctx context.Context, cutoff time.Time, msg string) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE video_runs SET status = 'failed', error_message = $2, message = $2,
			finished_at = NOW(), updated_at = NOW()
		WHERE status = 'scripting' AND updated_at < $1`, cutoff, msg)
	if err != nil {
		return 0, fmt.Errorf("video_repo: fail stale scripting: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
