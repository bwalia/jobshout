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

// BlogRepository persists blog_runs and their generated articles.
type BlogRepository interface {
	Create(ctx context.Context, run *model.BlogRun) error
	Update(ctx context.Context, run *model.BlogRun) error
	// UpdateSteps persists just the progress trace. Called on every step
	// transition, so it is deliberately narrow — a full Update would race with
	// the terminal write that happens at the end of a run.
	UpdateSteps(ctx context.Context, runID uuid.UUID, steps []model.BlogStep) error
	// UpdateBriefs persists what a run is writing about, for runs that did not
	// know at creation time.
	//
	// A trending run is created with no topics and discovers them once it
	// starts, and Update deliberately does not touch briefs — it is the
	// terminal write, and a run's subject is not something it should be able to
	// change on completion. So discovery gets its own narrow writer, for the
	// same reason UpdateSteps has one.
	UpdateBriefs(ctx context.Context, runID uuid.UUID, briefs []model.BlogBrief, topics []string) error
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

	// RecentTopics lists the topics this org has published articles about since
	// the given time, most recent first.
	//
	// It exists for topic discovery: a daily schedule that proposes whatever is
	// trending will keep proposing the same story for as long as it trends, and
	// this is how that gets filtered. Added with the schema it depends on
	// (migration 022 creates the supporting index) rather than left to the
	// feature that will consume it.
	RecentTopics(ctx context.Context, orgID uuid.UUID, since time.Time) ([]string, error)
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
	id, org_id, agent_id, triggered_by, source, status, topics, briefs, model,
	cms_namespace, articles, steps, error_message,
	started_at, completed_at, published_at, created_at`

// scanBlogRun reads one row in blogRunColumns order.
func scanBlogRun(row pgx.Row) (*model.BlogRun, error) {
	run := &model.BlogRun{}
	var topicsRaw, briefsRaw, articlesRaw, stepsRaw []byte
	err := row.Scan(
		&run.ID, &run.OrgID, &run.AgentID, &run.TriggeredBy, &run.Source, &run.Status,
		&topicsRaw, &briefsRaw, &run.Model, &run.CMSNamespace,
		&articlesRaw, &stepsRaw, &run.ErrorMessage,
		&run.StartedAt, &run.CompletedAt, &run.PublishedAt, &run.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(topicsRaw, &run.Topics)
	_ = json.Unmarshal(briefsRaw, &run.Briefs)
	_ = json.Unmarshal(articlesRaw, &run.Articles)
	_ = json.Unmarshal(stepsRaw, &run.Steps)

	// Migration 022 backfills briefs for rows that predate the column, but a
	// run created between that migration running and this code deploying — or
	// one written by an older instance during a rolling deploy — can still
	// arrive with topics and no briefs. Deriving them here means no caller ever
	// has to handle a run with an empty brief list.
	if len(run.Briefs) == 0 && len(run.Topics) > 0 {
		run.Briefs = make([]model.BlogBrief, 0, len(run.Topics))
		for _, t := range run.Topics {
			run.Briefs = append(run.Briefs, model.BlogBrief{Topic: t})
		}
	}
	return run, nil
}

// blogArticleColumns pairs with scanBlogArticle, for the same reason
// blogRunColumns pairs with scanBlogRun.
const blogArticleColumns = `
	id, run_id, org_id, topic, title, slug, path, references_json, markdown, html,
	post_uuid, post_status, posted_at, word_count, created_at`

// scanBlogArticle reads one row in blogArticleColumns order.
func scanBlogArticle(row pgx.Row) (*model.BlogArticle, error) {
	var a model.BlogArticle
	// title is nullable: articles written before it existed whose markdown had
	// no H1 for the backfill to find have none.
	var title *string
	var referencesRaw []byte
	err := row.Scan(
		&a.ID, &a.RunID, &a.OrgID, &a.Topic, &title, &a.Slug, &a.Path, &referencesRaw,
		&a.Markdown, &a.HTML,
		&a.PostUUID, &a.PostStatus, &a.PostedAt, &a.WordCount, &a.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	if title != nil {
		a.Title = *title
	}
	// Fall back to the topic so the UI always has something to show, matching
	// what articleTitle did when the title was derived on every read.
	if a.Title == "" {
		a.Title = a.Topic
	}
	_ = json.Unmarshal(referencesRaw, &a.References)
	if a.References == nil {
		a.References = []model.BlogReference{}
	}
	return &a, nil
}

func (r *blogRepository) Create(ctx context.Context, run *model.BlogRun) error {
	topicsJSON, _ := json.Marshal(run.Topics)
	briefsJSON, _ := json.Marshal(run.Briefs)
	articlesJSON, _ := json.Marshal(run.Articles)
	stepsJSON, _ := json.Marshal(run.Steps)

	const sql = `
		INSERT INTO blog_runs
		    (id, org_id, agent_id, triggered_by, source, status, topics, briefs, model, articles, steps, started_at, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12, NOW())
		RETURNING created_at`

	return r.pool.QueryRow(ctx, sql,
		run.ID, run.OrgID, run.AgentID, run.TriggeredBy, run.Source, run.Status,
		topicsJSON, briefsJSON, run.Model, articlesJSON, stepsJSON, run.StartedAt,
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

func (r *blogRepository) UpdateBriefs(
	ctx context.Context, runID uuid.UUID, briefs []model.BlogBrief, topics []string,
) error {
	briefsJSON, err := json.Marshal(briefs)
	if err != nil {
		return fmt.Errorf("blog_repo: marshal briefs: %w", err)
	}
	topicsJSON, err := json.Marshal(topics)
	if err != nil {
		return fmt.Errorf("blog_repo: marshal topics: %w", err)
	}

	// Both columns move together: topics is the topic-only projection of
	// briefs, and letting them disagree would give the legacy readers a
	// different answer than the current ones.
	const sql = `UPDATE blog_runs SET briefs = $2, topics = $3 WHERE id = $1`
	if _, err := r.pool.Exec(ctx, sql, runID, briefsJSON, topicsJSON); err != nil {
		return fmt.Errorf("blog_repo: update briefs: %w", err)
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
		INSERT INTO blog_articles
		    (id, run_id, org_id, topic, title, slug, path, references_json, markdown, html, word_count, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11, NOW())`
	for _, a := range articles {
		refsJSON, err := json.Marshal(a.References)
		if err != nil {
			return fmt.Errorf("blog_repo: marshal references for %q: %w", a.Slug, err)
		}
		batch.Queue(sql, a.ID, a.RunID, a.OrgID, a.Topic, a.Title, a.Slug, a.Path,
			refsJSON, a.Markdown, a.HTML, a.WordCount)
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

func (r *blogRepository) RecentTopics(ctx context.Context, orgID uuid.UUID, since time.Time) ([]string, error) {
	// Two sources, unioned, because an article is written minutes after the
	// topic is chosen.
	//
	// blog_articles alone is what a finished piece looks like — but a run that
	// has picked a subject and is still writing it has no article yet, so a
	// second run starting in that window sees nothing and picks the same thing.
	// That is not hypothetical: two runs 44 seconds apart produced the same
	// article during testing. A claimed topic has to count as taken from the
	// moment it is claimed.
	//
	// Failed runs are excluded on purpose. Their subject was attempted and not
	// delivered, so it should be available again rather than locked out for a
	// fortnight by a run that produced nothing.
	//
	// DISTINCT ON collapses repeats of the same topic to its most recent
	// mention, so a subject seen three times contributes one row. The outer
	// ordering is what the caller asked for; the inner one is what DISTINCT ON
	// requires to pick which duplicate survives.
	const sql = `
		SELECT topic FROM (
		    SELECT DISTINCT ON (topic) topic, created_at FROM (
		        SELECT topic, created_at
		        FROM blog_articles
		        WHERE org_id = $1 AND created_at >= $2

		        UNION ALL

		        SELECT b->>'topic' AS topic, r.created_at
		        FROM blog_runs r, jsonb_array_elements(r.briefs) b
		        WHERE r.org_id = $1
		          AND r.created_at >= $2
		          AND r.status <> 'failed'
		          AND COALESCE(b->>'topic', '') <> ''
		    ) all_topics
		    ORDER BY topic, created_at DESC
		) t
		ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, sql, orgID, since)
	if err != nil {
		return nil, fmt.Errorf("blog_repo: recent topics: %w", err)
	}
	defer rows.Close()

	topics := make([]string, 0)
	for rows.Next() {
		var topic string
		if err := rows.Scan(&topic); err != nil {
			return nil, fmt.Errorf("blog_repo: scan recent topic: %w", err)
		}
		topics = append(topics, topic)
	}
	return topics, rows.Err()
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
