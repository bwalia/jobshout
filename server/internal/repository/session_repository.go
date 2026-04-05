package repository

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

type SessionRepository interface {
	Create(ctx context.Context, s *model.Session) error
	GetByID(ctx context.Context, id uuid.UUID) (*model.Session, error)
	List(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Session], error)
	Update(ctx context.Context, id uuid.UUID, req model.UpdateSessionRequest) (*model.Session, error)
	Delete(ctx context.Context, id uuid.UUID) error
	AppendMessages(ctx context.Context, id uuid.UUID, msgs []model.SessionMsg, tokensDelta int) error
	CreateSnapshot(ctx context.Context, snap *model.SessionSnapshot) error
	ListSnapshots(ctx context.Context, sessionID uuid.UUID) ([]model.SessionSnapshot, error)
	GetSnapshot(ctx context.Context, id uuid.UUID) (*model.SessionSnapshot, error)
}

type sessionRepository struct {
	pool *pgxpool.Pool
}

func NewSessionRepository(pool *pgxpool.Pool) SessionRepository {
	return &sessionRepository{pool: pool}
}

func (r *sessionRepository) Create(ctx context.Context, s *model.Session) error {
	msgsJSON, _ := json.Marshal(s.ContextMessages)
	if msgsJSON == nil {
		msgsJSON = []byte("[]")
	}

	const sql = `
		INSERT INTO sessions
		    (id, org_id, name, description, provider_config_id, model_name,
		     status, context_messages, total_tokens, message_count, tags, created_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
		RETURNING created_at, updated_at`

	return r.pool.QueryRow(ctx, sql,
		s.ID, s.OrgID, s.Name, s.Description, s.ProviderConfigID, s.ModelName,
		s.Status, msgsJSON, s.TotalTokens, s.MessageCount, s.Tags, s.CreatedBy,
	).Scan(&s.CreatedAt, &s.UpdatedAt)
}

func (r *sessionRepository) GetByID(ctx context.Context, id uuid.UUID) (*model.Session, error) {
	const sql = `
		SELECT id, org_id, name, description, provider_config_id, model_name,
		       status, context_messages, total_tokens, message_count, tags,
		       created_by, created_at, updated_at
		FROM sessions WHERE id = $1`

	s := &model.Session{}
	var msgsRaw []byte
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&s.ID, &s.OrgID, &s.Name, &s.Description, &s.ProviderConfigID, &s.ModelName,
		&s.Status, &msgsRaw, &s.TotalTokens, &s.MessageCount, &s.Tags,
		&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("session_repo: get by id: %w", err)
	}
	_ = json.Unmarshal(msgsRaw, &s.ContextMessages)
	return s, nil
}

func (r *sessionRepository) List(ctx context.Context, orgID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.Session], error) {
	params.Normalize()

	var total int
	if err := r.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM sessions WHERE org_id = $1 AND status != 'deleted'", orgID,
	).Scan(&total); err != nil {
		return nil, fmt.Errorf("session_repo: count: %w", err)
	}

	const sql = `
		SELECT id, org_id, name, description, provider_config_id, model_name,
		       status, context_messages, total_tokens, message_count, tags,
		       created_by, created_at, updated_at
		FROM sessions WHERE org_id = $1 AND status != 'deleted'
		ORDER BY updated_at DESC LIMIT $2 OFFSET $3`

	rows, err := r.pool.Query(ctx, sql, orgID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("session_repo: list: %w", err)
	}
	defer rows.Close()

	var sessions []model.Session
	for rows.Next() {
		var s model.Session
		var msgsRaw []byte
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.Name, &s.Description, &s.ProviderConfigID, &s.ModelName,
			&s.Status, &msgsRaw, &s.TotalTokens, &s.MessageCount, &s.Tags,
			&s.CreatedBy, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("session_repo: scan: %w", err)
		}
		_ = json.Unmarshal(msgsRaw, &s.ContextMessages)
		sessions = append(sessions, s)
	}

	totalPages := (total + params.PerPage - 1) / params.PerPage
	return &model.PaginatedResponse[model.Session]{
		Data: sessions, Total: total, Page: params.Page, PerPage: params.PerPage, TotalPages: totalPages,
	}, rows.Err()
}

func (r *sessionRepository) Update(ctx context.Context, id uuid.UUID, req model.UpdateSessionRequest) (*model.Session, error) {
	const sql = `
		UPDATE sessions SET
		    name = COALESCE($2, name),
		    description = COALESCE($3, description),
		    status = COALESCE($4, status),
		    model_name = COALESCE($5, model_name),
		    updated_at = NOW()
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, sql, id, req.Name, req.Description, req.Status, req.ModelName)
	if err != nil {
		return nil, fmt.Errorf("session_repo: update: %w", err)
	}

	// Handle provider_config_id separately since it's a UUID pointer
	if req.ProviderConfigID != nil {
		provID, err := uuid.Parse(*req.ProviderConfigID)
		if err == nil {
			_, _ = r.pool.Exec(ctx,
				"UPDATE sessions SET provider_config_id = $2, updated_at = NOW() WHERE id = $1",
				id, provID)
		}
	}

	return r.GetByID(ctx, id)
}

func (r *sessionRepository) Delete(ctx context.Context, id uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		"UPDATE sessions SET status = 'deleted', updated_at = NOW() WHERE id = $1", id)
	return err
}

func (r *sessionRepository) AppendMessages(ctx context.Context, id uuid.UUID, msgs []model.SessionMsg, tokensDelta int) error {
	msgsJSON, _ := json.Marshal(msgs)

	const sql = `
		UPDATE sessions SET
		    context_messages = context_messages || $2::jsonb,
		    total_tokens = total_tokens + $3,
		    message_count = message_count + $4,
		    updated_at = NOW()
		WHERE id = $1`

	_, err := r.pool.Exec(ctx, sql, id, msgsJSON, tokensDelta, len(msgs))
	return err
}

func (r *sessionRepository) CreateSnapshot(ctx context.Context, snap *model.SessionSnapshot) error {
	msgsJSON, _ := json.Marshal(snap.ContextMessages)
	if msgsJSON == nil {
		msgsJSON = []byte("[]")
	}

	const sql = `
		INSERT INTO session_snapshots
		    (id, session_id, name, description, context_messages, provider_type, model_name, total_tokens, message_count)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING created_at`

	return r.pool.QueryRow(ctx, sql,
		snap.ID, snap.SessionID, snap.Name, snap.Description,
		msgsJSON, snap.ProviderType, snap.ModelName, snap.TotalTokens, snap.MessageCount,
	).Scan(&snap.CreatedAt)
}

func (r *sessionRepository) ListSnapshots(ctx context.Context, sessionID uuid.UUID) ([]model.SessionSnapshot, error) {
	const sql = `
		SELECT id, session_id, name, description, context_messages, provider_type, model_name,
		       total_tokens, message_count, created_at
		FROM session_snapshots WHERE session_id = $1 ORDER BY created_at DESC`

	rows, err := r.pool.Query(ctx, sql, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var snaps []model.SessionSnapshot
	for rows.Next() {
		var s model.SessionSnapshot
		var msgsRaw []byte
		if err := rows.Scan(
			&s.ID, &s.SessionID, &s.Name, &s.Description, &msgsRaw,
			&s.ProviderType, &s.ModelName, &s.TotalTokens, &s.MessageCount, &s.CreatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(msgsRaw, &s.ContextMessages)
		snaps = append(snaps, s)
	}
	return snaps, rows.Err()
}

func (r *sessionRepository) GetSnapshot(ctx context.Context, id uuid.UUID) (*model.SessionSnapshot, error) {
	const sql = `
		SELECT id, session_id, name, description, context_messages, provider_type, model_name,
		       total_tokens, message_count, created_at
		FROM session_snapshots WHERE id = $1`

	s := &model.SessionSnapshot{}
	var msgsRaw []byte
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&s.ID, &s.SessionID, &s.Name, &s.Description, &msgsRaw,
		&s.ProviderType, &s.ModelName, &s.TotalTokens, &s.MessageCount, &s.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal(msgsRaw, &s.ContextMessages)
	return s, nil
}
