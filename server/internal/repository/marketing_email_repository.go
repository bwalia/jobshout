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

// ErrMarketingEmailRunNotFound is returned for a run that does not exist in
// the caller's org.
var ErrMarketingEmailRunNotFound = errors.New("marketing email: run not found")

// MarketingEmailRunRepository stores Marketing Email Writer runs.
type MarketingEmailRunRepository interface {
	Create(ctx context.Context, run *model.MarketingEmailRun) error
	// Save writes every field that changes as a run moves.
	Save(ctx context.Context, run *model.MarketingEmailRun) error
	Get(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error)
	List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MarketingEmailRun, error)
	// FailStale fails runs still writing or publishing since before cutoff:
	// their goroutine died with its server. Returns how many.
	FailStale(ctx context.Context, cutoff time.Time, msg string) (int, error)
}

type marketingEmailRunRepository struct {
	pool *pgxpool.Pool
}

// NewMarketingEmailRunRepository constructs the repository.
func NewMarketingEmailRunRepository(pool *pgxpool.Pool) MarketingEmailRunRepository {
	return &marketingEmailRunRepository{pool: pool}
}

const marketingEmailRunCols = `id, org_id, task_id, agent_id, requested_by, brief, subject, preheader, title,
	slug, status, stage, message, error_message, content_html, draft_json, hero_image_url, page_uuid,
	page_status, browser_url, destination, model, publish_mode, created_at, started_at, finished_at, updated_at`

func scanMarketingEmailRun(row pgx.Row) (*model.MarketingEmailRun, error) {
	var (
		r            model.MarketingEmailRun
		brief, draft []byte
	)
	if err := row.Scan(&r.ID, &r.OrgID, &r.TaskID, &r.AgentID, &r.RequestedBy, &brief, &r.Subject,
		&r.Preheader, &r.Title, &r.Slug, &r.Status, &r.Stage, &r.Message, &r.Error, &r.ContentHTML, &draft,
		&r.HeroImageURL, &r.PageUUID, &r.PageStatus, &r.BrowserURL, &r.Destination, &r.Model, &r.PublishMode,
		&r.CreatedAt, &r.StartedAt, &r.FinishedAt, &r.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMarketingEmailRunNotFound
		}
		return nil, fmt.Errorf("marketing_email_repo: scan: %w", err)
	}
	_ = json.Unmarshal(brief, &r.Brief)
	_ = json.Unmarshal(draft, &r.Draft)
	if r.Draft.Sections == nil {
		r.Draft.Sections = []model.MarketingEmailSection{}
	}
	return &r, nil
}

func (r *marketingEmailRunRepository) Create(ctx context.Context, run *model.MarketingEmailRun) error {
	brief, err := json.Marshal(run.Brief)
	if err != nil {
		return err
	}
	return r.pool.QueryRow(ctx, `
		INSERT INTO marketing_email_runs (id, org_id, task_id, agent_id, requested_by, brief, slug, status, stage,
			message, destination, model, publish_mode, started_at, created_at, updated_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,NOW(),NOW())
		RETURNING created_at, updated_at`,
		run.ID, run.OrgID, run.TaskID, run.AgentID, run.RequestedBy, brief, run.Slug, run.Status, run.Stage,
		run.Message, run.Destination, run.Model, run.PublishMode, run.StartedAt,
	).Scan(&run.CreatedAt, &run.UpdatedAt)
}

func (r *marketingEmailRunRepository) Save(ctx context.Context, run *model.MarketingEmailRun) error {
	draft, err := json.Marshal(run.Draft)
	if err != nil {
		return err
	}
	_, err = r.pool.Exec(ctx, `
		UPDATE marketing_email_runs SET subject = $2, preheader = $3, title = $4, slug = $5, status = $6,
			stage = $7, message = $8, error_message = $9, content_html = $10, draft_json = $11,
			hero_image_url = $12, page_uuid = $13, page_status = $14, browser_url = $15, destination = $16,
			model = $17, publish_mode = $18, started_at = $19, finished_at = $20, updated_at = NOW()
		WHERE id = $1`,
		run.ID, run.Subject, run.Preheader, run.Title, run.Slug, run.Status, run.Stage, run.Message, run.Error,
		run.ContentHTML, draft, run.HeroImageURL, run.PageUUID, run.PageStatus, run.BrowserURL,
		run.Destination, run.Model, run.PublishMode, run.StartedAt, run.FinishedAt)
	if err != nil {
		return fmt.Errorf("marketing_email_repo: save: %w", err)
	}
	return nil
}

func (r *marketingEmailRunRepository) Get(ctx context.Context, orgID, id uuid.UUID) (*model.MarketingEmailRun, error) {
	return scanMarketingEmailRun(r.pool.QueryRow(ctx,
		`SELECT `+marketingEmailRunCols+` FROM marketing_email_runs WHERE org_id = $1 AND id = $2`, orgID, id))
}

func (r *marketingEmailRunRepository) List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.MarketingEmailRun, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.pool.Query(ctx, `SELECT `+marketingEmailRunCols+
		` FROM marketing_email_runs WHERE org_id = $1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("marketing_email_repo: list: %w", err)
	}
	defer rows.Close()
	out := []model.MarketingEmailRun{}
	for rows.Next() {
		run, err := scanMarketingEmailRun(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *run)
	}
	return out, rows.Err()
}

func (r *marketingEmailRunRepository) FailStale(ctx context.Context, cutoff time.Time, msg string) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		UPDATE marketing_email_runs SET status = 'failed', error_message = $2, message = $2,
			finished_at = NOW(), updated_at = NOW()
		WHERE status IN ('writing', 'publishing') AND updated_at < $1`, cutoff, msg)
	if err != nil {
		return 0, fmt.Errorf("marketing_email_repo: fail stale: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
