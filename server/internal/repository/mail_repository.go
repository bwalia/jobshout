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

// MailRepository persists Gmail connections, watched threads and drafts.
type MailRepository interface {
	UpsertConnection(ctx context.Context, c *model.MailConnection) error
	GetConnectionByOrg(ctx context.Context, orgID uuid.UUID) (*model.MailConnection, error)
	Disconnect(ctx context.Context, orgID uuid.UUID) error
	UpdateConnectionMeta(ctx context.Context, c *model.MailConnection) error

	PutOAuthState(ctx context.Context, state string, orgID, userID uuid.UUID, expires time.Time) error
	ConsumeOAuthState(ctx context.Context, state string) (orgID, userID uuid.UUID, err error)

	UpsertThread(ctx context.Context, t *model.MailThread) error
	GetThread(ctx context.Context, id uuid.UUID) (*model.MailThread, error)
	GetThreadByGmailID(ctx context.Context, orgID uuid.UUID, gmailThreadID string) (*model.MailThread, error)
	ListThreads(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailThread], error)
	UpdateThread(ctx context.Context, t *model.MailThread) error

	UpsertDraft(ctx context.Context, d *model.MailDraft) error
	GetDraft(ctx context.Context, id uuid.UUID) (*model.MailDraft, error)
	GetDraftByThread(ctx context.Context, threadID uuid.UUID) (*model.MailDraft, error)
	ListDraftsByStatus(ctx context.Context, orgID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailDraft], error)
	UpdateDraft(ctx context.Context, d *model.MailDraft) error

	ClaimDueConnections(ctx context.Context, limit int, lease time.Duration) ([]model.MailConnection, error)
	MarkSynced(ctx context.Context, id uuid.UUID, lastSync, nextSync time.Time) error
}

type mailRepository struct {
	pool *pgxpool.Pool
}

func NewMailRepository(pool *pgxpool.Pool) MailRepository {
	return &mailRepository{pool: pool}
}

const mailConnColumns = `
	id, org_id, agent_id, google_email, refresh_token_enc, token_expiry, scopes,
	allow_mailbox_mutations, watch_labels, watch_senders, watch_subject_prefixes,
	watch_knowledge_urls, knowledge_notes, research_focus, reply_instructions,
	status, status_error, last_sync_at, next_sync_at, sync_lease_until,
	connected_at, disconnected_at, created_at, updated_at`

func scanMailConn(row pgx.Row) (*model.MailConnection, error) {
	c := &model.MailConnection{}
	var scopes, labels, senders, prefixes, knowledge []string
	err := row.Scan(
		&c.ID, &c.OrgID, &c.AgentID, &c.GoogleEmail, &c.RefreshTokenEnc, &c.TokenExpiry, &scopes,
		&c.AllowMailboxMutations, &labels, &senders, &prefixes,
		&knowledge, &c.KnowledgeNotes, &c.ResearchFocus, &c.ReplyInstructions,
		&c.Status, &c.StatusError, &c.LastSyncAt, &c.NextSyncAt, &c.SyncLeaseUntil,
		&c.ConnectedAt, &c.DisconnectedAt, &c.CreatedAt, &c.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	c.Scopes = nzStrings(scopes)
	c.WatchLabels = nzStrings(labels)
	c.WatchSenders = nzStrings(senders)
	c.WatchSubjectPrefixes = nzStrings(prefixes)
	c.WatchKnowledgeURLs = nzStrings(knowledge)
	return c, nil
}

func nzStrings(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// upsertConnectionSQL writes sync_lease_until so a failed sync or EnqueueSync
// that nils the lease in memory actually clears it in postgres. Omitting the
// column left the 2-minute claim lease stuck after "mail: gmail api: decode".
const upsertConnectionSQL = `
		INSERT INTO mail_connections (
			id, org_id, agent_id, google_email, refresh_token_enc, token_expiry, scopes,
			allow_mailbox_mutations, watch_labels, watch_senders, watch_subject_prefixes,
			watch_knowledge_urls, knowledge_notes, research_focus, reply_instructions,
			status, status_error, last_sync_at, next_sync_at, sync_lease_until,
			connected_at, disconnected_at
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
		)
		ON CONFLICT (org_id) DO UPDATE SET
			agent_id = EXCLUDED.agent_id,
			google_email = EXCLUDED.google_email,
			refresh_token_enc = EXCLUDED.refresh_token_enc,
			token_expiry = EXCLUDED.token_expiry,
			scopes = EXCLUDED.scopes,
			allow_mailbox_mutations = EXCLUDED.allow_mailbox_mutations,
			watch_labels = EXCLUDED.watch_labels,
			watch_senders = EXCLUDED.watch_senders,
			watch_subject_prefixes = EXCLUDED.watch_subject_prefixes,
			watch_knowledge_urls = EXCLUDED.watch_knowledge_urls,
			knowledge_notes = EXCLUDED.knowledge_notes,
			research_focus = EXCLUDED.research_focus,
			reply_instructions = EXCLUDED.reply_instructions,
			status = EXCLUDED.status,
			status_error = EXCLUDED.status_error,
			last_sync_at = EXCLUDED.last_sync_at,
			next_sync_at = EXCLUDED.next_sync_at,
			sync_lease_until = EXCLUDED.sync_lease_until,
			connected_at = EXCLUDED.connected_at,
			disconnected_at = EXCLUDED.disconnected_at,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`

func (r *mailRepository) UpsertConnection(ctx context.Context, c *model.MailConnection) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}
	return r.pool.QueryRow(ctx, upsertConnectionSQL,
		c.ID, c.OrgID, c.AgentID, c.GoogleEmail, c.RefreshTokenEnc, c.TokenExpiry, nzStrings(c.Scopes),
		c.AllowMailboxMutations, nzStrings(c.WatchLabels), nzStrings(c.WatchSenders), nzStrings(c.WatchSubjectPrefixes),
		nzStrings(c.WatchKnowledgeURLs), c.KnowledgeNotes, c.ResearchFocus, c.ReplyInstructions,
		c.Status, c.StatusError, c.LastSyncAt, c.NextSyncAt, c.SyncLeaseUntil, c.ConnectedAt, c.DisconnectedAt,
	).Scan(&c.ID, &c.CreatedAt, &c.UpdatedAt)
}

func (r *mailRepository) GetConnectionByOrg(ctx context.Context, orgID uuid.UUID) (*model.MailConnection, error) {
	c, err := scanMailConn(r.pool.QueryRow(ctx, `SELECT `+mailConnColumns+` FROM mail_connections WHERE org_id = $1`, orgID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("mail_repo: get connection: %w", err)
	}
	return c, nil
}

func (r *mailRepository) Disconnect(ctx context.Context, orgID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE mail_connections SET
			refresh_token_enc = NULL,
			google_email = '',
			status = 'disconnected',
			status_error = NULL,
			disconnected_at = NOW(),
			next_sync_at = NULL,
			sync_lease_until = NULL,
			updated_at = NOW()
		WHERE org_id = $1`, orgID)
	if err != nil {
		return fmt.Errorf("mail_repo: disconnect: %w", err)
	}
	return nil
}

const updateConnectionMetaSQL = `
		UPDATE mail_connections SET
			allow_mailbox_mutations = $2,
			watch_labels = $3,
			watch_senders = $4,
			watch_subject_prefixes = $5,
			watch_knowledge_urls = $6,
			knowledge_notes = $7,
			research_focus = $8,
			reply_instructions = $9,
			updated_at = NOW()
		WHERE org_id = $1
		RETURNING updated_at`

func (r *mailRepository) UpdateConnectionMeta(ctx context.Context, c *model.MailConnection) error {
	return r.pool.QueryRow(ctx, updateConnectionMetaSQL,
		c.OrgID, c.AllowMailboxMutations, nzStrings(c.WatchLabels), nzStrings(c.WatchSenders), nzStrings(c.WatchSubjectPrefixes),
		nzStrings(c.WatchKnowledgeURLs), c.KnowledgeNotes, c.ResearchFocus, c.ReplyInstructions,
	).Scan(&c.UpdatedAt)
}

func (r *mailRepository) PutOAuthState(ctx context.Context, state string, orgID, userID uuid.UUID, expires time.Time) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO mail_oauth_states (state, org_id, user_id, expires_at)
		VALUES ($1,$2,$3,$4)`, state, orgID, userID, expires)
	if err != nil {
		return fmt.Errorf("mail_repo: put oauth state: %w", err)
	}
	return nil
}

func (r *mailRepository) ConsumeOAuthState(ctx context.Context, state string) (uuid.UUID, uuid.UUID, error) {
	var orgID, userID uuid.UUID
	var expires time.Time
	err := r.pool.QueryRow(ctx, `
		DELETE FROM mail_oauth_states WHERE state = $1
		RETURNING org_id, user_id, expires_at`, state).Scan(&orgID, &userID, &expires)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, uuid.Nil, fmt.Errorf("mail_repo: oauth state not found")
		}
		return uuid.Nil, uuid.Nil, fmt.Errorf("mail_repo: consume oauth state: %w", err)
	}
	if time.Now().After(expires) {
		return uuid.Nil, uuid.Nil, fmt.Errorf("mail_repo: oauth state expired")
	}
	return orgID, userID, nil
}

const mailThreadColumns = `
	id, org_id, agent_id, connection_id, gmail_thread_id, gmail_message_id,
	from_email, from_name, to_email, subject, snippet, body_text,
	message_id_header, references_header, received_at, status, classification,
	needs_research, research_summary, research_findings, research_brief_id,
	error_message, created_at, updated_at`

func scanMailThread(row pgx.Row) (*model.MailThread, error) {
	t := &model.MailThread{}
	var classJSON, findingsJSON []byte
	err := row.Scan(
		&t.ID, &t.OrgID, &t.AgentID, &t.ConnectionID, &t.GmailThreadID, &t.GmailMessageID,
		&t.FromEmail, &t.FromName, &t.ToEmail, &t.Subject, &t.Snippet, &t.BodyText,
		&t.MessageIDHeader, &t.ReferencesHeader, &t.ReceivedAt, &t.Status, &classJSON,
		&t.NeedsResearch, &t.ResearchSummary, &findingsJSON, &t.ResearchBriefID,
		&t.ErrorMessage, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	t.ResearchFindings = findingsJSON
	if len(classJSON) > 0 && string(classJSON) != "null" {
		var c model.MailClassification
		if json.Unmarshal(classJSON, &c) == nil {
			t.Classification = &c
		}
	}
	return t, nil
}

func classJSON(c *model.MailClassification) []byte {
	if c == nil {
		return nil
	}
	b, err := json.Marshal(c)
	if err != nil {
		return nil
	}
	return b
}

func (r *mailRepository) UpsertThread(ctx context.Context, t *model.MailThread) error {
	if t.ID == uuid.Nil {
		t.ID = uuid.New()
	}
	const sql = `
		INSERT INTO mail_threads (
			id, org_id, agent_id, connection_id, gmail_thread_id, gmail_message_id,
			from_email, from_name, to_email, subject, snippet, body_text,
			message_id_header, references_header, received_at, status, classification,
			needs_research, research_summary, research_findings, research_brief_id, error_message
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22
		)
		ON CONFLICT (org_id, gmail_thread_id) DO UPDATE SET
			gmail_message_id = EXCLUDED.gmail_message_id,
			from_email = EXCLUDED.from_email,
			from_name = EXCLUDED.from_name,
			to_email = EXCLUDED.to_email,
			subject = EXCLUDED.subject,
			snippet = EXCLUDED.snippet,
			body_text = EXCLUDED.body_text,
			message_id_header = EXCLUDED.message_id_header,
			references_header = EXCLUDED.references_header,
			received_at = EXCLUDED.received_at,
			updated_at = NOW()
		RETURNING id, status, created_at, updated_at`
	return r.pool.QueryRow(ctx, sql,
		t.ID, t.OrgID, t.AgentID, t.ConnectionID, t.GmailThreadID, t.GmailMessageID,
		t.FromEmail, t.FromName, t.ToEmail, t.Subject, t.Snippet, t.BodyText,
		t.MessageIDHeader, t.ReferencesHeader, t.ReceivedAt, t.Status, classJSON(t.Classification),
		t.NeedsResearch, t.ResearchSummary, t.ResearchFindings, t.ResearchBriefID, t.ErrorMessage,
	).Scan(&t.ID, &t.Status, &t.CreatedAt, &t.UpdatedAt)
}

func (r *mailRepository) GetThread(ctx context.Context, id uuid.UUID) (*model.MailThread, error) {
	t, err := scanMailThread(r.pool.QueryRow(ctx, `SELECT `+mailThreadColumns+` FROM mail_threads WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("mail_repo: get thread: %w", err)
	}
	return t, nil
}

func (r *mailRepository) GetThreadByGmailID(ctx context.Context, orgID uuid.UUID, gmailThreadID string) (*model.MailThread, error) {
	t, err := scanMailThread(r.pool.QueryRow(ctx,
		`SELECT `+mailThreadColumns+` FROM mail_threads WHERE org_id = $1 AND gmail_thread_id = $2`,
		orgID, gmailThreadID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("mail_repo: get thread by gmail: %w", err)
	}
	return t, nil
}

func (r *mailRepository) ListThreads(ctx context.Context, orgID uuid.UUID, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailThread], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM mail_threads WHERE org_id = $1`, orgID).Scan(&total); err != nil {
		return nil, fmt.Errorf("mail_repo: count threads: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+mailThreadColumns+`
		FROM mail_threads WHERE org_id = $1
		ORDER BY updated_at DESC
		LIMIT $2 OFFSET $3`, orgID, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, fmt.Errorf("mail_repo: list threads: %w", err)
	}
	defer rows.Close()
	data := []model.MailThread{}
	for rows.Next() {
		t, err := scanMailThread(rows)
		if err != nil {
			return nil, fmt.Errorf("mail_repo: scan thread: %w", err)
		}
		data = append(data, *t)
	}
	pages := 0
	if pagination.PerPage > 0 {
		pages = (total + pagination.PerPage - 1) / pagination.PerPage
	}
	return &model.PaginatedResponse[model.MailThread]{
		Data: data, Total: total, Page: pagination.Page, PerPage: pagination.PerPage, TotalPages: pages,
	}, rows.Err()
}

func (r *mailRepository) UpdateThread(ctx context.Context, t *model.MailThread) error {
	const sql = `
		UPDATE mail_threads SET
			status = $2, classification = $3, needs_research = $4,
			research_summary = $5, research_findings = $6, research_brief_id = $7,
			error_message = $8, updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	return r.pool.QueryRow(ctx, sql,
		t.ID, t.Status, classJSON(t.Classification), t.NeedsResearch,
		t.ResearchSummary, t.ResearchFindings, t.ResearchBriefID, t.ErrorMessage,
	).Scan(&t.UpdatedAt)
}

const mailDraftColumns = `
	id, org_id, thread_id, agent_id, status, subject, body, to_email, cc_email,
	research_brief_id, approved_by, approved_at, rejected_by, rejected_at,
	gmail_message_id, sent_at, created_at, updated_at`

func scanMailDraft(row pgx.Row) (*model.MailDraft, error) {
	d := &model.MailDraft{}
	err := row.Scan(
		&d.ID, &d.OrgID, &d.ThreadID, &d.AgentID, &d.Status, &d.Subject, &d.Body, &d.ToEmail, &d.CCEmail,
		&d.ResearchBriefID, &d.ApprovedBy, &d.ApprovedAt, &d.RejectedBy, &d.RejectedAt,
		&d.GmailMessageID, &d.SentAt, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return d, nil
}

func (r *mailRepository) UpsertDraft(ctx context.Context, d *model.MailDraft) error {
	if d.ID == uuid.Nil {
		d.ID = uuid.New()
	}
	const sql = `
		INSERT INTO mail_drafts (
			id, org_id, thread_id, agent_id, status, subject, body, to_email, cc_email, research_brief_id
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (thread_id) DO UPDATE SET
			subject = EXCLUDED.subject,
			body = EXCLUDED.body,
			to_email = EXCLUDED.to_email,
			cc_email = EXCLUDED.cc_email,
			research_brief_id = EXCLUDED.research_brief_id,
			status = EXCLUDED.status,
			updated_at = NOW()
		RETURNING id, created_at, updated_at`
	return r.pool.QueryRow(ctx, sql,
		d.ID, d.OrgID, d.ThreadID, d.AgentID, d.Status, d.Subject, d.Body, d.ToEmail, d.CCEmail, d.ResearchBriefID,
	).Scan(&d.ID, &d.CreatedAt, &d.UpdatedAt)
}

func (r *mailRepository) GetDraft(ctx context.Context, id uuid.UUID) (*model.MailDraft, error) {
	d, err := scanMailDraft(r.pool.QueryRow(ctx, `SELECT `+mailDraftColumns+` FROM mail_drafts WHERE id = $1`, id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("mail_repo: get draft: %w", err)
	}
	return d, nil
}

func (r *mailRepository) GetDraftByThread(ctx context.Context, threadID uuid.UUID) (*model.MailDraft, error) {
	d, err := scanMailDraft(r.pool.QueryRow(ctx, `SELECT `+mailDraftColumns+` FROM mail_drafts WHERE thread_id = $1`, threadID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("mail_repo: get draft by thread: %w", err)
	}
	return d, nil
}

func (r *mailRepository) ListDraftsByStatus(ctx context.Context, orgID uuid.UUID, status string, pagination model.PaginationParams) (*model.PaginatedResponse[model.MailDraft], error) {
	pagination.Normalize()
	var total int
	if err := r.pool.QueryRow(ctx, `SELECT COUNT(*) FROM mail_drafts WHERE org_id = $1 AND status = $2`, orgID, status).Scan(&total); err != nil {
		return nil, fmt.Errorf("mail_repo: count drafts: %w", err)
	}
	rows, err := r.pool.Query(ctx, `
		SELECT `+mailDraftColumns+`
		FROM mail_drafts WHERE org_id = $1 AND status = $2
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4`, orgID, status, pagination.PerPage, pagination.Offset())
	if err != nil {
		return nil, fmt.Errorf("mail_repo: list drafts: %w", err)
	}
	defer rows.Close()
	data := []model.MailDraft{}
	for rows.Next() {
		d, err := scanMailDraft(rows)
		if err != nil {
			return nil, fmt.Errorf("mail_repo: scan draft: %w", err)
		}
		data = append(data, *d)
	}
	pages := 0
	if pagination.PerPage > 0 {
		pages = (total + pagination.PerPage - 1) / pagination.PerPage
	}
	return &model.PaginatedResponse[model.MailDraft]{
		Data: data, Total: total, Page: pagination.Page, PerPage: pagination.PerPage, TotalPages: pages,
	}, rows.Err()
}

func (r *mailRepository) UpdateDraft(ctx context.Context, d *model.MailDraft) error {
	const sql = `
		UPDATE mail_drafts SET
			status = $2, subject = $3, body = $4, to_email = $5, cc_email = $6,
			research_brief_id = $7, approved_by = $8, approved_at = $9,
			rejected_by = $10, rejected_at = $11, gmail_message_id = $12, sent_at = $13,
			updated_at = NOW()
		WHERE id = $1
		RETURNING updated_at`
	return r.pool.QueryRow(ctx, sql,
		d.ID, d.Status, d.Subject, d.Body, d.ToEmail, d.CCEmail,
		d.ResearchBriefID, d.ApprovedBy, d.ApprovedAt,
		d.RejectedBy, d.RejectedAt, d.GmailMessageID, d.SentAt,
	).Scan(&d.UpdatedAt)
}

// claimDuePredicate is the eligibility filter for a mailbox sync. Status
// 'error' still has a refresh token; claiming only 'connected' left the inbox
// stuck after a single Gmail decode failure.
const claimDuePredicate = `
			status IN ('connected', 'error')
			  AND refresh_token_enc IS NOT NULL
			  AND (next_sync_at IS NULL OR next_sync_at <= NOW())
			  AND (sync_lease_until IS NULL OR sync_lease_until < NOW())`

func (r *mailRepository) ClaimDueConnections(ctx context.Context, limit int, lease time.Duration) ([]model.MailConnection, error) {
	if limit < 1 {
		limit = 5
	}
	leaseInterval := fmt.Sprintf("%d milliseconds", lease.Milliseconds())
	rows, err := r.pool.Query(ctx, `
		UPDATE mail_connections
		SET sync_lease_until = NOW() + $2::interval, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM mail_connections
			WHERE `+claimDuePredicate+`
			ORDER BY next_sync_at NULLS FIRST
			FOR UPDATE SKIP LOCKED
			LIMIT $1
		)
		RETURNING `+mailConnColumns, limit, leaseInterval)
	if err != nil {
		return nil, fmt.Errorf("mail_repo: claim connections: %w", err)
	}
	defer rows.Close()
	var out []model.MailConnection
	for rows.Next() {
		c, err := scanMailConn(rows)
		if err != nil {
			return nil, fmt.Errorf("mail_repo: scan claimed: %w", err)
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

func (r *mailRepository) MarkSynced(ctx context.Context, id uuid.UUID, lastSync, nextSync time.Time) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE mail_connections SET
			last_sync_at = $2, next_sync_at = $3, sync_lease_until = NULL,
			status = 'connected', status_error = NULL, updated_at = NOW()
		WHERE id = $1`, id, lastSync, nextSync)
	if err != nil {
		return fmt.Errorf("mail_repo: mark synced: %w", err)
	}
	return nil
}
