package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// ChatRepository manages persistence for chat sessions and messages.
type ChatRepository interface {
	CreateSession(ctx context.Context, session *model.ChatSession) error
	GetSession(ctx context.Context, id uuid.UUID) (*model.ChatSession, error)
	UpdateSession(ctx context.Context, id uuid.UUID, agentID *uuid.UUID) error
	ListSessions(ctx context.Context, orgID, userID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ChatSession], error)

	AppendMessage(ctx context.Context, msg *model.ChatMessage) error
	ListMessages(ctx context.Context, sessionID uuid.UUID, limit int) ([]model.ChatMessage, error)
}

type chatRepository struct {
	pool *pgxpool.Pool
}

func NewChatRepository(pool *pgxpool.Pool) ChatRepository {
	return &chatRepository{pool: pool}
}

func (r *chatRepository) CreateSession(ctx context.Context, session *model.ChatSession) error {
	if session.ID == uuid.Nil {
		session.ID = uuid.New()
	}
	metadata := session.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	const sql = `
		INSERT INTO chat_sessions (id, org_id, user_id, agent_id, source, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := r.pool.Exec(ctx, sql,
		session.ID, session.OrgID, session.UserID,
		session.AgentID, session.Source, metadata,
	)
	if err != nil {
		return fmt.Errorf("chat_repo: create_session: %w", err)
	}
	return nil
}

func (r *chatRepository) GetSession(ctx context.Context, id uuid.UUID) (*model.ChatSession, error) {
	const sql = `
		SELECT id, org_id, user_id, agent_id, source, metadata, created_at, updated_at
		FROM chat_sessions WHERE id = $1`

	var s model.ChatSession
	err := r.pool.QueryRow(ctx, sql, id).Scan(
		&s.ID, &s.OrgID, &s.UserID, &s.AgentID,
		&s.Source, &s.Metadata, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("chat_repo: get_session: %w", err)
	}
	return &s, nil
}

func (r *chatRepository) UpdateSession(ctx context.Context, id uuid.UUID, agentID *uuid.UUID) error {
	const sql = `UPDATE chat_sessions SET agent_id = $2, updated_at = NOW() WHERE id = $1`
	_, err := r.pool.Exec(ctx, sql, id, agentID)
	if err != nil {
		return fmt.Errorf("chat_repo: update_session: %w", err)
	}
	return nil
}

func (r *chatRepository) ListSessions(ctx context.Context, orgID, userID uuid.UUID, params model.PaginationParams) (*model.PaginatedResponse[model.ChatSession], error) {
	params.Normalize()

	const countSQL = `SELECT COUNT(*) FROM chat_sessions WHERE org_id = $1 AND user_id = $2`
	var total int
	if err := r.pool.QueryRow(ctx, countSQL, orgID, userID).Scan(&total); err != nil {
		return nil, fmt.Errorf("chat_repo: list_sessions count: %w", err)
	}

	const sql = `
		SELECT id, org_id, user_id, agent_id, source, metadata, created_at, updated_at
		FROM chat_sessions
		WHERE org_id = $1 AND user_id = $2
		ORDER BY updated_at DESC
		LIMIT $3 OFFSET $4`

	rows, err := r.pool.Query(ctx, sql, orgID, userID, params.PerPage, params.Offset())
	if err != nil {
		return nil, fmt.Errorf("chat_repo: list_sessions: %w", err)
	}
	defer rows.Close()

	var sessions []model.ChatSession
	for rows.Next() {
		var s model.ChatSession
		if err := rows.Scan(
			&s.ID, &s.OrgID, &s.UserID, &s.AgentID,
			&s.Source, &s.Metadata, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("chat_repo: list_sessions scan: %w", err)
		}
		sessions = append(sessions, s)
	}

	return &model.PaginatedResponse[model.ChatSession]{
		Data:    sessions,
		Total:   total,
		Page:    params.Page,
		PerPage: params.PerPage,
	}, nil
}

func (r *chatRepository) AppendMessage(ctx context.Context, msg *model.ChatMessage) error {
	if msg.ID == uuid.Nil {
		msg.ID = uuid.New()
	}
	metadata := msg.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	const sql = `
		INSERT INTO chat_messages (id, session_id, org_id, role, source, content, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err := r.pool.Exec(ctx, sql,
		msg.ID, msg.SessionID, msg.OrgID,
		msg.Role, msg.Source, msg.Content, metadata,
	)
	if err != nil {
		return fmt.Errorf("chat_repo: append_message: %w", err)
	}
	return nil
}

func (r *chatRepository) ListMessages(ctx context.Context, sessionID uuid.UUID, limit int) ([]model.ChatMessage, error) {
	if limit <= 0 {
		limit = 50
	}
	const sql = `
		SELECT id, session_id, org_id, role, source, content, metadata, created_at
		FROM chat_messages
		WHERE session_id = $1
		ORDER BY created_at ASC
		LIMIT $2`

	rows, err := r.pool.Query(ctx, sql, sessionID, limit)
	if err != nil {
		return nil, fmt.Errorf("chat_repo: list_messages: %w", err)
	}
	defer rows.Close()

	var messages []model.ChatMessage
	for rows.Next() {
		var m model.ChatMessage
		if err := rows.Scan(
			&m.ID, &m.SessionID, &m.OrgID, &m.Role, &m.Source, &m.Content, &m.Metadata, &m.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("chat_repo: list_messages scan: %w", err)
		}
		messages = append(messages, m)
	}
	return messages, nil
}
