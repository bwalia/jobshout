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

// BlogRepository persists blog_runs and their generated articles.
type BlogRepository interface {
	Create(ctx context.Context, run *model.BlogRun) error
	Update(ctx context.Context, run *model.BlogRun) error
	// UpdateSteps persists just the progress trace. Called on every step
	// transition, so it is deliberately narrow — a full Update would race with
	// the terminal write that happens at the end of a run.
	UpdateSteps(ctx context.Context, runID uuid.UUID, steps []model.BlogStep) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error)
	ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error)
	// Delete removes a run. Its articles go with it via ON DELETE CASCADE.
	Delete(ctx context.Context, id uuid.UUID) error

	CreateArticles(ctx context.Context, articles []model.BlogArticle) error
	ListArticlesByRun(ctx context.Context, runID uuid.UUID) ([]model.BlogArticle, error)
	GetArticle(ctx context.Context, id uuid.UUID) (*model.BlogArticle, error)
	// MarkArticlesPosted records where each article landed in the CMS, after a
	// publish has already succeeded.
	MarkArticlesPosted(ctx context.Context, posts []model.BlogArticlePost) error
	// DeleteArticlesByRun clears a run's articles so a retry cannot leave the
	// previous attempt's output alongside the new one.
	DeleteArticlesByRun(ctx context.Context, runID uuid.UUID) error
}

type blogRepository struct {
	pool *pgxpool.Pool
}

// NewBlogRepository creates a BlogRepository backed by pgxpool.
func NewBlogRepository(pool *pgxpool.Pool) BlogRepository {
	return &blogRepository{pool: pool}
}

// blogRunColumns is the shared SELECT list so the row scanners cannot drift.
const blogRunColumns = `
	id, org_id, agent_id, triggered_by, source, status, topics, model,
	cms_namespace, articles, steps, error_message,
	started_at, completed_at, published_at, created_at`

// scanBlogRun reads one row in blogRunColumns order.
func scanBlogRun(row pgx.Row) (*model.BlogRun, error) {
	run := &model.BlogRun{}
	var topicsRaw, articlesRaw, stepsRaw []byte
	err := row.Scan(
		&run.ID, &run.OrgID, &run.AgentID, &run.TriggeredBy, &run.Source, &run.Status,
		&topicsRaw, &run.Model, &run.CMSNamespace,
		&articlesRaw, &stepsRaw, &run.ErrorMessage,
		&run.StartedAt, &run.CompletedAt, &run.PublishedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(topicsRaw, &run.Topics)
	_ = json.Unmarshal(articlesRaw, &run.Articles)
	_ = json.Unmarshal(stepsRaw, &run.Steps)
	return run, nil
}

// blogArticleColumns pairs with scanBlogArticle, for the same reason
// blogRunColumns pairs with scanBlogRun.
const blogArticleColumns = `
	id, run_id, org_id, topic, slug, path, markdown, html,
	post_uuid, post_status, posted_at, word_count, created_at`

// scanBlogArticle reads one row in blogArticleColumns order.
func scanBlogArticle(row pgx.Row) (*model.BlogArticle, error) {
	var a model.BlogArticle
	err := row.Scan(
		&a.ID, &a.RunID, &a.OrgID, &a.Topic, &a.Slug, &a.Path, &a.Markdown, &a.HTML,
		&a.PostUUID, &a.PostStatus, &a.PostedAt, &a.WordCount, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

func (r *blogRepository) Create(ctx context.Context, run *model.BlogRun) error {
	topicsJSON, _ := json.Marshal(run.Topics)
	articlesJSON, _ := json.Marshal(run.Articles)
	stepsJSON, _ := json.Marshal(run.Steps)

	const sql = `
		INSERT INTO blog_runs
		    (id, org_id, agent_id, triggered_by, source, status, topics, model, articles, steps, started_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, NOW())
		RETURNING created_at`

	return r.pool.QueryRow(ctx, sql,
		run.ID, run.OrgID, run.AgentID, run.TriggeredBy, run.Source, run.Status,
		topicsJSON, run.Model, articlesJSON, stepsJSON, run.StartedAt,
	).Scan(&run.CreatedAt)
}

func (r *blogRepository) Update(ctx context.Context, run *model.BlogRun) error {
	articlesJSON, _ := json.Marshal(run.Articles)
	stepsJSON, _ := json.Marshal(run.Steps)

	const sql = `
		UPDATE blog_runs SET
		    status        = $2,
		    cms_namespace = $3,
		    articles      = $4,
		    steps         = $5,
		    error_message = $6,
		    completed_at  = $7,
		    published_at  = $8
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, sql,
		run.ID, run.Status, run.CMSNamespace,
		articlesJSON, stepsJSON, run.ErrorMessage, run.CompletedAt, run.PublishedAt,
	)
	if err != nil {
		return fmt.Errorf("blog_repo: update: %w", err)
	}
	return nil
}

func (r *blogRepository) UpdateSteps(ctx context.Context, runID uuid.UUID, steps []model.BlogStep) error {
	stepsJSON, _ := json.Marshal(steps)
	_, err := r.pool.Exec(ctx, `UPDATE blog_runs SET steps = $2 WHERE id = $1`, runID, stepsJSON)
	if err != nil {
		return fmt.Errorf("blog_repo: update steps: %w", err)
	}
	return nil
}

func (r *blogRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.BlogRun, error) {
	sql := `SELECT ` + blogRunColumns + ` FROM blog_runs WHERE id = $1`
	run, err := scanBlogRun(r.pool.QueryRow(ctx, sql, id))
	if err != nil {
		return nil, fmt.Errorf("blog_repo: get: %w", err)
	}
	return run, nil
}

func (r *blogRepository) Delete(ctx context.Context, id uuid.UUID) error {
	// blog_articles.run_id is ON DELETE CASCADE, so the bodies go with the run.
	tag, err := r.pool.Exec(ctx, `DELETE FROM blog_runs WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("blog_repo: delete run: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("blog_repo: run not found")
	}
	return nil
}

func (r *blogRepository) DeleteArticlesByRun(ctx context.Context, runID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM blog_articles WHERE run_id = $1`, runID); err != nil {
		return fmt.Errorf("blog_repo: delete articles: %w", err)
	}
	return nil
}

func (r *blogRepository) ListByOrg(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.BlogRun], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx, "SELECT COUNT(*) FROM blog_runs WHERE org_id = $1", orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("blog_repo: count: %w", err)
	}

	sql := `SELECT ` + blogRunColumns + `
		FROM blog_runs WHERE org_id = $1
		ORDER BY created_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("blog_repo: list: %w", err)
	}
	defer rows.Close()

	runs := make([]model.BlogRun, 0)
	for rows.Next() {
		run, err := scanBlogRun(rows)
		if err != nil {
			return nil, fmt.Errorf("blog_repo: scan: %w", err)
		}
		runs = append(runs, *run)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.BlogRun]{
		Data: runs, Total: total, Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, rows.Err()
}

func (r *blogRepository) CreateArticles(ctx context.Context, articles []model.BlogArticle) error {
	if len(articles) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	const sql = `
		INSERT INTO blog_articles (id, run_id, org_id, topic, slug, path, markdown, html, word_count, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9, NOW())`
	for _, a := range articles {
		batch.Queue(sql, a.ID, a.RunID, a.OrgID, a.Topic, a.Slug, a.Path, a.Markdown, a.HTML, a.WordCount)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range articles {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("blog_repo: insert article: %w", err)
		}
	}
	return nil
}

func (r *blogRepository) ListArticlesByRun(ctx context.Context, runID uuid.UUID) ([]model.BlogArticle, error) {
	sql := `SELECT ` + blogArticleColumns + `
		FROM blog_articles WHERE run_id = $1 ORDER BY created_at ASC`

	rows, err := r.pool.Query(ctx, sql, runID)
	if err != nil {
		return nil, fmt.Errorf("blog_repo: list articles: %w", err)
	}
	defer rows.Close()

	articles := make([]model.BlogArticle, 0)
	for rows.Next() {
		a, err := scanBlogArticle(rows)
		if err != nil {
			return nil, fmt.Errorf("blog_repo: scan article: %w", err)
		}
		articles = append(articles, *a)
	}
	return articles, rows.Err()
}

func (r *blogRepository) GetArticle(ctx context.Context, id uuid.UUID) (*model.BlogArticle, error) {
	sql := `SELECT ` + blogArticleColumns + ` FROM blog_articles WHERE id = $1`

	a, err := scanBlogArticle(r.pool.QueryRow(ctx, sql, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("blog_repo: get article: %w", err)
	}
	return a, nil
}

func (r *blogRepository) MarkArticlesPosted(ctx context.Context, posts []model.BlogArticlePost) error {
	if len(posts) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	const sql = `
		UPDATE blog_articles
		SET post_uuid = $2, post_status = $3, posted_at = NOW()
		WHERE id = $1`
	for _, p := range posts {
		batch.Queue(sql, p.ArticleID, p.PostUUID, p.Status)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()
	for range posts {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("blog_repo: mark article posted: %w", err)
		}
	}
	return nil
}
