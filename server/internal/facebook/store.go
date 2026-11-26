package facebook

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store is the Facebook Page Manager's persistence.
type Store struct{ pool *pgxpool.Pool }

// NewStore constructs the store.
func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

const postCols = `id, org_id, page_id, task_id, status, message, image_url, image_prompt, category, audience,
	objective, notes, scheduled_for, error_message, fb_post_id, post_url, published_at, approved_by,
	reactions, comments, shares, stats_at, created_at, updated_at`

func scanPost(row pgx.Row) (*Post, error) {
	var p Post
	err := row.Scan(&p.ID, &p.OrgID, &p.PageID, &p.TaskID, &p.Status, &p.Message, &p.ImageURL, &p.ImagePrompt,
		&p.Category, &p.Audience, &p.Objective, &p.Notes, &p.ScheduledFor, &p.ErrorMessage, &p.FBPostID, &p.PostURL,
		&p.PublishedAt, &p.ApprovedBy, &p.Reactions, &p.Comments, &p.Shares, &p.StatsAt, &p.CreatedAt, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("facebook_store: scan post: %w", err)
	}
	return &p, nil
}

func (s *Store) queryPosts(ctx context.Context, sql string, args ...any) ([]Post, error) {
	rows, err := s.pool.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("facebook_store: query posts: %w", err)
	}
	defer rows.Close()
	var out []Post
	for rows.Next() {
		p, err := scanPost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// ── Posts ───────────────────────────────────────────────────────────────────

// InsertDrafting creates n placeholder rows in drafting for a calendar run.
func (s *Store) InsertDrafting(ctx context.Context, orgID uuid.UUID, pageID string, taskID *uuid.UUID, notes string, n int) ([]Post, error) {
	var out []Post
	for i := 0; i < n; i++ {
		p, err := scanPost(s.pool.QueryRow(ctx, `INSERT INTO facebook_posts (org_id, page_id, task_id, status, notes)
			VALUES ($1,$2,$3,'drafting',$4) RETURNING `+postCols, orgID, pageID, taskID, notes))
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, nil
}

// FillDraft stores a drafted post (or the error that ended it).
func (s *Store) FillDraft(ctx context.Context, id uuid.UUID, d Draft, errMsg string) error {
	status := StatusDraft
	if errMsg != "" {
		status = StatusFailed
	}
	_, err := s.pool.Exec(ctx, `UPDATE facebook_posts SET status=$2, message=$3, image_prompt=$4, category=$5,
		audience=$6, objective=$7, scheduled_for=$8, error_message=$9, updated_at=NOW()
		WHERE id=$1 AND status='drafting'`,
		id, status, d.Message, d.ImagePrompt, d.Category, d.Audience, d.Objective, d.SuggestedFor, errMsg)
	return err
}

// SetImage stores a creative's URL.
func (s *Store) SetImage(ctx context.Context, orgID, id uuid.UUID, url string) error {
	_, err := s.pool.Exec(ctx, `UPDATE facebook_posts SET image_url=$3, updated_at=NOW()
		WHERE org_id=$1 AND id=$2 AND status IN ('drafting','draft','failed','scheduled')`, orgID, id, url)
	return err
}

// FailStaleDrafting fails drafting rows whose drafter died with its pod.
func (s *Store) FailStaleDrafting(ctx context.Context, before time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE facebook_posts SET status='failed',
		error_message='drafting was interrupted; redraft it', updated_at=NOW()
		WHERE status='drafting' AND updated_at < $1`, before)
	return int(tag.RowsAffected()), err
}

// FailStalePublishing returns posts stuck in publishing (a pod died mid-call)
// to draft with a warning. They are not retried: Facebook may have published.
func (s *Store) FailStalePublishing(ctx context.Context, before time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE facebook_posts SET status='draft',
		error_message='publishing was interrupted — check the Page before posting again', updated_at=NOW()
		WHERE status='publishing' AND updated_at < $1`, before)
	return int(tag.RowsAffected()), err
}

// ListPosts lists an org's posts, newest first.
func (s *Store) ListPosts(ctx context.Context, orgID uuid.UUID, pageID string, limit int) ([]Post, error) {
	return s.queryPosts(ctx, `SELECT `+postCols+` FROM facebook_posts
		WHERE org_id=$1 AND ($2='' OR page_id=$2)
		ORDER BY COALESCE(scheduled_for, created_at) DESC, created_at DESC LIMIT $3`, orgID, pageID, limit)
}

// GetPost returns one post in the org.
func (s *Store) GetPost(ctx context.Context, orgID, id uuid.UUID) (*Post, error) {
	return scanPost(s.pool.QueryRow(ctx, `SELECT `+postCols+` FROM facebook_posts WHERE org_id=$1 AND id=$2`, orgID, id))
}

// UpdateDraft edits a post that has not been published. Editing a scheduled
// post returns it to draft so a change is always re-approved.
func (s *Store) UpdateDraft(ctx context.Context, orgID, id uuid.UUID, message string, at *time.Time, clearAt bool) (*Post, error) {
	p, err := scanPost(s.pool.QueryRow(ctx, `UPDATE facebook_posts SET message=$3,
		scheduled_for = CASE WHEN $5 THEN NULL WHEN $4::timestamptz IS NOT NULL THEN $4 ELSE scheduled_for END,
		status='draft', error_message='', updated_at=NOW()
		WHERE org_id=$1 AND id=$2 AND status IN ('draft','failed','scheduled')
		RETURNING `+postCols, orgID, id, message, at, clearAt))
	if errors.Is(err, ErrNotFound) {
		return nil, s.missingOrConflict(ctx, orgID, id)
	}
	return p, err
}

// Schedule approves a draft for publication at when.
func (s *Store) Schedule(ctx context.Context, orgID, id uuid.UUID, when time.Time, by *uuid.UUID) (*Post, error) {
	p, err := scanPost(s.pool.QueryRow(ctx, `UPDATE facebook_posts SET status='scheduled', scheduled_for=$3,
		approved_by=$4, error_message='', updated_at=NOW()
		WHERE org_id=$1 AND id=$2 AND status IN ('draft','failed','scheduled') AND message <> ''
		RETURNING `+postCols, orgID, id, when, by))
	if errors.Is(err, ErrNotFound) {
		return nil, s.missingOrConflict(ctx, orgID, id)
	}
	return p, err
}

// Unschedule returns a scheduled post to draft.
func (s *Store) Unschedule(ctx context.Context, orgID, id uuid.UUID) (*Post, error) {
	p, err := scanPost(s.pool.QueryRow(ctx, `UPDATE facebook_posts SET status='draft', updated_at=NOW()
		WHERE org_id=$1 AND id=$2 AND status='scheduled' RETURNING `+postCols, orgID, id))
	if errors.Is(err, ErrNotFound) {
		return nil, s.missingOrConflict(ctx, orgID, id)
	}
	return p, err
}

// ClaimNow moves a draft, failed or scheduled post to publishing. One caller wins.
func (s *Store) ClaimNow(ctx context.Context, orgID, id uuid.UUID, by *uuid.UUID) (*Post, error) {
	p, err := scanPost(s.pool.QueryRow(ctx, `UPDATE facebook_posts SET status='publishing',
		approved_by=COALESCE($3, approved_by), error_message='', updated_at=NOW()
		WHERE org_id=$1 AND id=$2 AND status IN ('draft','failed','scheduled') AND message <> ''
		RETURNING `+postCols, orgID, id, by))
	if errors.Is(err, ErrNotFound) {
		return nil, s.missingOrConflict(ctx, orgID, id)
	}
	return p, err
}

// ClaimDue claims scheduled posts whose time has come. SKIP LOCKED means two
// replicas ticking together never take the same row.
func (s *Store) ClaimDue(ctx context.Context, now time.Time, limit int) ([]Post, error) {
	return s.queryPosts(ctx, `UPDATE facebook_posts SET status='publishing', updated_at=NOW()
		WHERE id IN (SELECT p.id FROM facebook_posts p WHERE p.status='scheduled' AND p.scheduled_for <= $1
		             AND NOT EXISTS (SELECT 1 FROM facebook_pages g WHERE g.org_id=p.org_id AND g.page_id=p.page_id AND g.paused)
		             ORDER BY p.scheduled_for FOR UPDATE OF p SKIP LOCKED LIMIT $2)
		RETURNING `+postCols, now, limit)
}

// MarkPublished records a successful publish.
func (s *Store) MarkPublished(ctx context.Context, id uuid.UUID, fbID, url string) (*Post, error) {
	return scanPost(s.pool.QueryRow(ctx, `UPDATE facebook_posts SET status='published', fb_post_id=$2, post_url=$3,
		published_at=NOW(), error_message='', updated_at=NOW() WHERE id=$1 RETURNING `+postCols, id, fbID, url))
}

// ReleasePublishing returns a post Facebook rejected to draft, with the reason.
func (s *Store) ReleasePublishing(ctx context.Context, id uuid.UUID, errMsg string) error {
	_, err := s.pool.Exec(ctx, `UPDATE facebook_posts SET status='draft', error_message=$2, updated_at=NOW()
		WHERE id=$1 AND status='publishing'`, id, errMsg)
	return err
}

// DeletePost removes a post row.
func (s *Store) DeletePost(ctx context.Context, orgID, id uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM facebook_posts WHERE org_id=$1 AND id=$2 AND status <> 'publishing'`, orgID, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return s.missingOrConflict(ctx, orgID, id)
	}
	return nil
}

// ResetForRedraft puts a post back in drafting.
func (s *Store) ResetForRedraft(ctx context.Context, orgID, id uuid.UUID) (*Post, error) {
	p, err := scanPost(s.pool.QueryRow(ctx, `UPDATE facebook_posts SET status='drafting', error_message='', updated_at=NOW()
		WHERE org_id=$1 AND id=$2 AND status IN ('draft','failed') RETURNING `+postCols, orgID, id))
	if errors.Is(err, ErrNotFound) {
		return nil, s.missingOrConflict(ctx, orgID, id)
	}
	return p, err
}

// SetStats stores a published post's engagement counts.
func (s *Store) SetStats(ctx context.Context, id uuid.UUID, st PostStatsRow) error {
	_, err := s.pool.Exec(ctx, `UPDATE facebook_posts SET reactions=$2, comments=$3, shares=$4, stats_at=NOW() WHERE id=$1`,
		id, st.Reactions, st.Comments, st.Shares)
	return err
}

// PostStatsRow is the counts SetStats stores.
type PostStatsRow struct{ Reactions, Comments, Shares int }

// Published lists a Page's published posts, newest first.
func (s *Store) Published(ctx context.Context, orgID uuid.UUID, pageID string, limit int) ([]Post, error) {
	return s.queryPosts(ctx, `SELECT `+postCols+` FROM facebook_posts
		WHERE org_id=$1 AND page_id=$2 AND status='published' AND fb_post_id <> ''
		ORDER BY published_at DESC LIMIT $3`, orgID, pageID, limit)
}

// Recent lists a Page's latest posts of any status, for the drafter to avoid
// repeating them.
func (s *Store) Recent(ctx context.Context, orgID uuid.UUID, pageID string, limit int) ([]Post, error) {
	return s.queryPosts(ctx, `SELECT `+postCols+` FROM facebook_posts
		WHERE org_id=$1 AND page_id=$2 AND status IN ('draft','scheduled','published') AND message <> ''
		ORDER BY created_at DESC LIMIT $3`, orgID, pageID, limit)
}

func (s *Store) missingOrConflict(ctx context.Context, orgID, id uuid.UUID) error {
	var one int
	err := s.pool.QueryRow(ctx, `SELECT 1 FROM facebook_posts WHERE org_id=$1 AND id=$2`, orgID, id).Scan(&one)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	return ErrConflict
}

// ── Pages and connection ────────────────────────────────────────────────────

const pageCols = `id, org_id, page_id, name, category, about, description, website, followers, needs_reauth,
	brief, voice, timezone, paused, updated_at`

func scanPage(row pgx.Row) (*PageRecord, error) {
	var p PageRecord
	err := row.Scan(&p.ID, &p.OrgID, &p.PageID, &p.Name, &p.Category, &p.About, &p.Description, &p.Website,
		&p.Followers, &p.NeedsReauth, &p.Brief, &p.Voice, &p.Timezone, &p.Paused, &p.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("facebook_store: scan page: %w", err)
	}
	return &p, nil
}

// UpsertPage stores a Page and its sealed token, keeping the Page's profile.
func (s *Store) UpsertPage(ctx context.Context, orgID uuid.UUID, p Page, tokenEnc []byte) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO facebook_pages (org_id, page_id, name, category, about, description,
		website, followers, token_enc, needs_reauth)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,FALSE)
		ON CONFLICT (org_id, page_id) DO UPDATE SET name=EXCLUDED.name, category=EXCLUDED.category,
		about=EXCLUDED.about, description=EXCLUDED.description, website=EXCLUDED.website,
		followers=EXCLUDED.followers, token_enc=EXCLUDED.token_enc, needs_reauth=FALSE, updated_at=NOW()`,
		orgID, p.ID, p.Name, p.Category, p.About, p.Description, p.Website, p.Followers, tokenEnc)
	return err
}

// ListPages lists an org's Pages.
func (s *Store) ListPages(ctx context.Context, orgID uuid.UUID) ([]PageRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+pageCols+` FROM facebook_pages WHERE org_id=$1 ORDER BY name`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PageRecord{}
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetPage returns a Page by its Facebook ID, with its sealed token.
func (s *Store) GetPage(ctx context.Context, orgID uuid.UUID, pageID string) (*PageRecord, []byte, error) {
	var enc []byte
	row := s.pool.QueryRow(ctx, `SELECT `+pageCols+`, token_enc FROM facebook_pages WHERE org_id=$1 AND page_id=$2`, orgID, pageID)
	var p PageRecord
	err := row.Scan(&p.ID, &p.OrgID, &p.PageID, &p.Name, &p.Category, &p.About, &p.Description, &p.Website,
		&p.Followers, &p.NeedsReauth, &p.Brief, &p.Voice, &p.Timezone, &p.Paused, &p.UpdatedAt, &enc)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	return &p, enc, nil
}

// SetProfile saves a Page's brief, voice and timezone.
func (s *Store) SetProfile(ctx context.Context, orgID uuid.UUID, pageID, brief, voice, tz string) (*PageRecord, error) {
	return scanPage(s.pool.QueryRow(ctx, `UPDATE facebook_pages SET brief=$3, voice=$4, timezone=$5, updated_at=NOW()
		WHERE org_id=$1 AND page_id=$2 RETURNING `+pageCols, orgID, pageID, brief, voice, tz))
}

// SetPaused pauses or resumes publishing for a Page.
func (s *Store) SetPaused(ctx context.Context, orgID uuid.UUID, pageID string, paused bool) (*PageRecord, error) {
	return scanPage(s.pool.QueryRow(ctx, `UPDATE facebook_pages SET paused=$3, updated_at=NOW()
		WHERE org_id=$1 AND page_id=$2 RETURNING `+pageCols, orgID, pageID, paused))
}

// SetPageInfo refreshes the details read from Facebook.
func (s *Store) SetPageInfo(ctx context.Context, orgID uuid.UUID, p Page) error {
	_, err := s.pool.Exec(ctx, `UPDATE facebook_pages SET name=$3, category=$4, about=$5, description=$6, website=$7,
		followers=$8, updated_at=NOW() WHERE org_id=$1 AND page_id=$2`,
		orgID, p.ID, p.Name, p.Category, p.About, p.Description, p.Website, p.Followers)
	return err
}

// MarkNeedsReauth flags a Page whose token Meta rejected.
func (s *Store) MarkNeedsReauth(ctx context.Context, orgID uuid.UUID, pageID string) {
	_, _ = s.pool.Exec(ctx, `UPDATE facebook_pages SET needs_reauth=TRUE, updated_at=NOW() WHERE org_id=$1 AND page_id=$2`, orgID, pageID)
}

// UpsertConnection stores who connected.
func (s *Store) UpsertConnection(ctx context.Context, c Connection) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO facebook_connections (org_id, fb_user_id, name, connected_by, expires_at)
		VALUES ($1,$2,$3,$4,$5)
		ON CONFLICT (org_id) DO UPDATE SET fb_user_id=EXCLUDED.fb_user_id, name=EXCLUDED.name,
		connected_by=EXCLUDED.connected_by, expires_at=EXCLUDED.expires_at, updated_at=NOW()`,
		c.OrgID, c.FBUserID, c.Name, c.ConnectedBy, c.ExpiresAt)
	return err
}

// GetConnection returns the org's connection.
func (s *Store) GetConnection(ctx context.Context, orgID uuid.UUID) (*Connection, error) {
	var c Connection
	err := s.pool.QueryRow(ctx, `SELECT org_id, fb_user_id, name, connected_by, expires_at FROM facebook_connections WHERE org_id=$1`, orgID).
		Scan(&c.OrgID, &c.FBUserID, &c.Name, &c.ConnectedBy, &c.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

// Disconnect removes the connection and every stored Page token. Posts and
// drafts are kept.
func (s *Store) Disconnect(ctx context.Context, orgID uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx) //nolint:errcheck
	if _, err := tx.Exec(ctx, `DELETE FROM facebook_pages WHERE org_id=$1`, orgID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM facebook_connections WHERE org_id=$1`, orgID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// PutOAuthState stores a one-time connect state.
func (s *Store) PutOAuthState(ctx context.Context, state string, orgID, userID uuid.UUID, expires time.Time) error {
	_, _ = s.pool.Exec(ctx, `DELETE FROM facebook_oauth_states WHERE expires_at < NOW()`)
	_, err := s.pool.Exec(ctx, `INSERT INTO facebook_oauth_states (state, org_id, user_id, expires_at) VALUES ($1,$2,$3,$4)`,
		state, orgID, userID, expires)
	return err
}

// ConsumeOAuthState returns and deletes a state; an expired or used one is an error.
func (s *Store) ConsumeOAuthState(ctx context.Context, state string) (uuid.UUID, uuid.UUID, error) {
	var orgID, userID uuid.UUID
	err := s.pool.QueryRow(ctx, `DELETE FROM facebook_oauth_states WHERE state=$1 AND expires_at > NOW()
		RETURNING org_id, user_id`, state).Scan(&orgID, &userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, uuid.Nil, ErrNotFound
	}
	return orgID, userID, err
}
