package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// ErrLinkedInNotFound is returned for a post, article or connection that does
// not exist in the caller's org.
var ErrLinkedInNotFound = errors.New("linkedin: not found")

// ErrLinkedInConflict is returned when a post is not in a state that allows
// the change (editing a posted post, posting one that is already posting).
var ErrLinkedInConflict = errors.New("linkedin: post is not in a state that allows this")

// LinkedInRepository stores the LinkedIn Poster's connection and posts, and
// reads the published articles it drafts from.
type LinkedInRepository interface {
	// FindArticle returns a published article in the org.
	FindArticle(ctx context.Context, orgID, articleID uuid.UUID) (*model.LinkedInArticle, error)
	// FindArticleByTitle returns the most recently published article whose
	// title contains words.
	FindArticleByTitle(ctx context.Context, orgID uuid.UUID, words string) (*model.LinkedInArticle, error)
	// LatestUndrafted returns the most recently published article with no
	// LinkedIn drafts.
	LatestUndrafted(ctx context.Context, orgID uuid.UUID) (*model.LinkedInArticle, error)
	// PublishedSince lists articles published after since with no LinkedIn
	// drafts, oldest first.
	PublishedSince(ctx context.Context, orgID uuid.UUID, since time.Time, limit int) ([]model.LinkedInArticle, error)

	// ClaimDrafts inserts a drafting row per variant and returns the rows it
	// claimed. A variant that already has a row is skipped, unless redraft is
	// set and that row is a draft or failed, in which case it is reset.
	// Posted and in-flight rows are never claimed.
	ClaimDrafts(ctx context.Context, p model.LinkedInPost, variants []string, redraft bool) ([]model.LinkedInPost, error)
	// FinishDraft stores the text of a drafting row (or the error that ended it).
	FinishDraft(ctx context.Context, id uuid.UUID, commentary, errMsg string) error
	// FailStaleDrafting fails drafting rows not updated since before, whose
	// drafter died with its pod. Returns how many.
	FailStaleDrafting(ctx context.Context, before time.Time) (int, error)

	ListPosts(ctx context.Context, orgID uuid.UUID, limit int) ([]model.LinkedInPost, error)
	GetPost(ctx context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error)
	// UpdateCommentary edits a draft (or a failed row, which becomes a draft).
	UpdateCommentary(ctx context.Context, orgID, id uuid.UUID, commentary string) (*model.LinkedInPost, error)
	// ClaimPosting moves a draft to posting. Only one caller can win.
	ClaimPosting(ctx context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error)
	MarkPosted(ctx context.Context, id uuid.UUID, urn, url string, by *uuid.UUID) (*model.LinkedInPost, error)
	// ReleasePosting returns a post LinkedIn rejected to draft, with the reason.
	ReleasePosting(ctx context.Context, id uuid.UUID, errMsg string) error

	GetConnection(ctx context.Context, orgID uuid.UUID) (*model.LinkedInConnection, []byte, error)
	UpsertConnection(ctx context.Context, c model.LinkedInConnection, tokenEnc []byte) error
	DeleteConnection(ctx context.Context, orgID uuid.UUID) error
	// ConnectedOrgs lists orgs with an unexpired connection.
	ConnectedOrgs(ctx context.Context) ([]uuid.UUID, error)

	PutOAuthState(ctx context.Context, state string, orgID, userID uuid.UUID, expires time.Time) error
	ConsumeOAuthState(ctx context.Context, state string) (orgID, userID uuid.UUID, err error)
}

type linkedInRepository struct {
	pool *pgxpool.Pool
}

// NewLinkedInRepository constructs the repository.
func NewLinkedInRepository(pool *pgxpool.Pool) LinkedInRepository {
	return &linkedInRepository{pool: pool}
}

// linkedInPublishedAt is when an article went public: on jobshout.com
// Insights, or in the CMS. NULL means it is not published.
const linkedInPublishedAt = `COALESCE(
	CASE WHEN LOWER(COALESCE(a.insights_status, '')) = 'published' THEN a.insights_posted_at END,
	CASE WHEN COALESCE(a.post_status, '') = 'published' THEN a.posted_at END)`

const linkedInArticleCols = `a.id, a.org_id, COALESCE(NULLIF(a.title, ''), a.topic), COALESCE(a.markdown, ''),
	COALESCE(a.audience, ''), CASE WHEN LOWER(COALESCE(a.insights_status, '')) = 'published' THEN COALESCE(a.insights_slug, '') ELSE '' END,
	` + linkedInPublishedAt

func scanLinkedInArticle(row pgx.Row) (*model.LinkedInArticle, error) {
	var a model.LinkedInArticle
	if err := row.Scan(&a.ID, &a.OrgID, &a.Title, &a.Markdown, &a.Audience, &a.InsightsSlug, &a.PublishedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLinkedInNotFound
		}
		return nil, fmt.Errorf("linkedin_repo: article: %w", err)
	}
	return &a, nil
}

const linkedInUndrafted = `NOT EXISTS (SELECT 1 FROM linkedin_posts p WHERE p.article_id = a.id)`

func (r *linkedInRepository) FindArticle(ctx context.Context, orgID, articleID uuid.UUID) (*model.LinkedInArticle, error) {
	return scanLinkedInArticle(r.pool.QueryRow(ctx, `SELECT `+linkedInArticleCols+`
		FROM blog_articles a WHERE a.org_id = $1 AND a.id = $2 AND `+linkedInPublishedAt+` IS NOT NULL`,
		orgID, articleID))
}

func (r *linkedInRepository) FindArticleByTitle(ctx context.Context, orgID uuid.UUID, words string) (*model.LinkedInArticle, error) {
	return scanLinkedInArticle(r.pool.QueryRow(ctx, `SELECT `+linkedInArticleCols+`
		FROM blog_articles a
		WHERE a.org_id = $1 AND `+linkedInPublishedAt+` IS NOT NULL
		  AND COALESCE(NULLIF(a.title, ''), a.topic) ILIKE '%' || $2 || '%'
		ORDER BY `+linkedInPublishedAt+` DESC LIMIT 1`,
		orgID, words))
}

func (r *linkedInRepository) LatestUndrafted(ctx context.Context, orgID uuid.UUID) (*model.LinkedInArticle, error) {
	return scanLinkedInArticle(r.pool.QueryRow(ctx, `SELECT `+linkedInArticleCols+`
		FROM blog_articles a
		WHERE a.org_id = $1 AND `+linkedInPublishedAt+` IS NOT NULL AND `+linkedInUndrafted+`
		ORDER BY `+linkedInPublishedAt+` DESC LIMIT 1`,
		orgID))
}

func (r *linkedInRepository) PublishedSince(ctx context.Context, orgID uuid.UUID, since time.Time, limit int) ([]model.LinkedInArticle, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+linkedInArticleCols+`
		FROM blog_articles a
		WHERE a.org_id = $1 AND `+linkedInPublishedAt+` > $2 AND `+linkedInUndrafted+`
		ORDER BY `+linkedInPublishedAt+` ASC LIMIT $3`,
		orgID, since, limit)
	if err != nil {
		return nil, fmt.Errorf("linkedin_repo: published since: %w", err)
	}
	defer rows.Close()
	var out []model.LinkedInArticle
	for rows.Next() {
		a, err := scanLinkedInArticle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *a)
	}
	return out, rows.Err()
}

const linkedInPostCols = `id, org_id, article_id, article_title, task_id, variant, status, commentary, link_url, notes,
	error_message, post_urn, post_url, posted_at, posted_by, created_at, updated_at`

func scanLinkedInPost(row pgx.Row) (*model.LinkedInPost, error) {
	var p model.LinkedInPost
	if err := row.Scan(&p.ID, &p.OrgID, &p.ArticleID, &p.ArticleTitle, &p.TaskID, &p.Variant, &p.Status,
		&p.Commentary, &p.LinkURL, &p.Notes, &p.ErrorMessage, &p.PostURN, &p.PostURL, &p.PostedAt, &p.PostedBy,
		&p.CreatedAt, &p.UpdatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrLinkedInNotFound
		}
		return nil, fmt.Errorf("linkedin_repo: post: %w", err)
	}
	return &p, nil
}

func (r *linkedInRepository) ClaimDrafts(ctx context.Context, p model.LinkedInPost, variants []string, redraft bool) ([]model.LinkedInPost, error) {
	// On conflict, a redraft resets a draft or failed row; anything else
	// (drafting, posting, posted, or any row when not redrafting) is left
	// alone, and RETURNING then yields nothing for that variant.
	onConflict := `DO NOTHING`
	if redraft {
		onConflict = `DO UPDATE SET status = 'drafting', commentary = '', error_message = '',
			notes = EXCLUDED.notes, link_url = EXCLUDED.link_url, task_id = EXCLUDED.task_id,
			article_title = EXCLUDED.article_title, updated_at = NOW()
			WHERE linkedin_posts.status IN ('draft', 'failed')`
	}
	var out []model.LinkedInPost
	for _, v := range variants {
		claimed, err := scanLinkedInPost(r.pool.QueryRow(ctx, `
			INSERT INTO linkedin_posts (org_id, article_id, article_title, task_id, variant, status, link_url, notes)
			VALUES ($1, $2, $3, $4, $5, 'drafting', $6, $7)
			ON CONFLICT (article_id, variant) `+onConflict+`
			RETURNING `+linkedInPostCols,
			p.OrgID, p.ArticleID, p.ArticleTitle, p.TaskID, v, p.LinkURL, p.Notes))
		if errors.Is(err, ErrLinkedInNotFound) {
			continue
		}
		if err != nil {
			return out, err
		}
		out = append(out, *claimed)
	}
	return out, nil
}

func (r *linkedInRepository) FinishDraft(ctx context.Context, id uuid.UUID, commentary, errMsg string) error {
	status := model.LinkedInPostDraft
	if errMsg != "" {
		status = model.LinkedInPostFailed
	}
	_, err := r.pool.Exec(ctx, `UPDATE linkedin_posts
		SET status = $2, commentary = $3, error_message = $4, updated_at = NOW()
		WHERE id = $1 AND status = 'drafting'`, id, status, commentary, errMsg)
	if err != nil {
		return fmt.Errorf("linkedin_repo: finish draft: %w", err)
	}
	return nil
}

func (r *linkedInRepository) FailStaleDrafting(ctx context.Context, before time.Time) (int, error) {
	tag, err := r.pool.Exec(ctx, `UPDATE linkedin_posts
		SET status = 'failed', error_message = 'drafting was interrupted (server restart); redraft it', updated_at = NOW()
		WHERE status = 'drafting' AND updated_at < $1`, before)
	if err != nil {
		return 0, fmt.Errorf("linkedin_repo: fail stale drafting: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (r *linkedInRepository) ListPosts(ctx context.Context, orgID uuid.UUID, limit int) ([]model.LinkedInPost, error) {
	rows, err := r.pool.Query(ctx, `SELECT `+linkedInPostCols+` FROM linkedin_posts
		WHERE org_id = $1 ORDER BY created_at DESC, variant LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, fmt.Errorf("linkedin_repo: list posts: %w", err)
	}
	defer rows.Close()
	out := []model.LinkedInPost{}
	for rows.Next() {
		p, err := scanLinkedInPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *linkedInRepository) GetPost(ctx context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error) {
	return scanLinkedInPost(r.pool.QueryRow(ctx, `SELECT `+linkedInPostCols+` FROM linkedin_posts
		WHERE org_id = $1 AND id = $2`, orgID, id))
}

// conflictOrMissing tells a guarded update that matched nothing apart: the
// post exists but is in the wrong state, or it does not exist in this org.
func (r *linkedInRepository) conflictOrMissing(ctx context.Context, orgID, id uuid.UUID) error {
	if _, err := r.GetPost(ctx, orgID, id); err != nil {
		return err
	}
	return ErrLinkedInConflict
}

func (r *linkedInRepository) UpdateCommentary(ctx context.Context, orgID, id uuid.UUID, commentary string) (*model.LinkedInPost, error) {
	p, err := scanLinkedInPost(r.pool.QueryRow(ctx, `UPDATE linkedin_posts
		SET commentary = $3, status = 'draft', error_message = '', updated_at = NOW()
		WHERE org_id = $1 AND id = $2 AND status IN ('draft', 'failed')
		RETURNING `+linkedInPostCols, orgID, id, commentary))
	if errors.Is(err, ErrLinkedInNotFound) {
		return nil, r.conflictOrMissing(ctx, orgID, id)
	}
	return p, err
}

func (r *linkedInRepository) ClaimPosting(ctx context.Context, orgID, id uuid.UUID) (*model.LinkedInPost, error) {
	p, err := scanLinkedInPost(r.pool.QueryRow(ctx, `UPDATE linkedin_posts
		SET status = 'posting', error_message = '', updated_at = NOW()
		WHERE org_id = $1 AND id = $2 AND status = 'draft' AND commentary <> ''
		RETURNING `+linkedInPostCols, orgID, id))
	if errors.Is(err, ErrLinkedInNotFound) {
		return nil, r.conflictOrMissing(ctx, orgID, id)
	}
	return p, err
}

func (r *linkedInRepository) MarkPosted(ctx context.Context, id uuid.UUID, urn, url string, by *uuid.UUID) (*model.LinkedInPost, error) {
	return scanLinkedInPost(r.pool.QueryRow(ctx, `UPDATE linkedin_posts
		SET status = 'posted', post_urn = $2, post_url = $3, posted_by = $4, posted_at = NOW(), error_message = '', updated_at = NOW()
		WHERE id = $1 AND status = 'posting'
		RETURNING `+linkedInPostCols, id, urn, url, by))
}

func (r *linkedInRepository) ReleasePosting(ctx context.Context, id uuid.UUID, errMsg string) error {
	_, err := r.pool.Exec(ctx, `UPDATE linkedin_posts
		SET status = 'draft', error_message = $2, updated_at = NOW()
		WHERE id = $1 AND status = 'posting'`, id, errMsg)
	if err != nil {
		return fmt.Errorf("linkedin_repo: release posting: %w", err)
	}
	return nil
}

func (r *linkedInRepository) GetConnection(ctx context.Context, orgID uuid.UUID) (*model.LinkedInConnection, []byte, error) {
	var c model.LinkedInConnection
	var enc []byte
	err := r.pool.QueryRow(ctx, `SELECT org_id, member_urn, name, connected_by, expires_at, created_at, updated_at, access_token_enc
		FROM linkedin_connections WHERE org_id = $1`, orgID).
		Scan(&c.OrgID, &c.MemberURN, &c.Name, &c.ConnectedBy, &c.ExpiresAt, &c.CreatedAt, &c.UpdatedAt, &enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrLinkedInNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("linkedin_repo: get connection: %w", err)
	}
	return &c, enc, nil
}

func (r *linkedInRepository) UpsertConnection(ctx context.Context, c model.LinkedInConnection, tokenEnc []byte) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO linkedin_connections (org_id, member_urn, name, access_token_enc, expires_at, connected_by)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (org_id) DO UPDATE SET member_urn = EXCLUDED.member_urn, name = EXCLUDED.name,
			access_token_enc = EXCLUDED.access_token_enc, expires_at = EXCLUDED.expires_at,
			connected_by = EXCLUDED.connected_by, updated_at = NOW()`,
		c.OrgID, c.MemberURN, c.Name, tokenEnc, c.ExpiresAt, c.ConnectedBy)
	if err != nil {
		return fmt.Errorf("linkedin_repo: upsert connection: %w", err)
	}
	return nil
}

func (r *linkedInRepository) DeleteConnection(ctx context.Context, orgID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM linkedin_connections WHERE org_id = $1`, orgID); err != nil {
		return fmt.Errorf("linkedin_repo: delete connection: %w", err)
	}
	return nil
}

func (r *linkedInRepository) ConnectedOrgs(ctx context.Context) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT org_id FROM linkedin_connections WHERE expires_at > NOW()`)
	if err != nil {
		return nil, fmt.Errorf("linkedin_repo: connected orgs: %w", err)
	}
	defer rows.Close()
	var out []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

func (r *linkedInRepository) PutOAuthState(ctx context.Context, state string, orgID, userID uuid.UUID, expires time.Time) error {
	// Expired states are swept on every write; there are only ever a few.
	if _, err := r.pool.Exec(ctx, `DELETE FROM linkedin_oauth_states WHERE expires_at < NOW()`); err != nil {
		return fmt.Errorf("linkedin_repo: sweep oauth states: %w", err)
	}
	_, err := r.pool.Exec(ctx, `INSERT INTO linkedin_oauth_states (state, org_id, user_id, expires_at) VALUES ($1, $2, $3, $4)`,
		state, orgID, userID, expires)
	if err != nil {
		return fmt.Errorf("linkedin_repo: put oauth state: %w", err)
	}
	return nil
}

func (r *linkedInRepository) ConsumeOAuthState(ctx context.Context, state string) (uuid.UUID, uuid.UUID, error) {
	var orgID, userID uuid.UUID
	err := r.pool.QueryRow(ctx, `DELETE FROM linkedin_oauth_states WHERE state = $1 AND expires_at > NOW()
		RETURNING org_id, user_id`, state).Scan(&orgID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrLinkedInNotFound
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, fmt.Errorf("linkedin_repo: consume oauth state: %w", err)
	}
	return orgID, userID, nil
}
