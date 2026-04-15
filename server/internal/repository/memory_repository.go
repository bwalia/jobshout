package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// MemoryRepository manages agent short-term and long-term memory persistence.
type MemoryRepository interface {
	UpsertShortTerm(ctx context.Context, mem *model.AgentMemoryShortTerm) error
	GetShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) (*model.AgentMemoryShortTerm, error)
	DeleteShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) error

	AppendLongTerm(ctx context.Context, mem *model.AgentMemoryLongTerm) error
	SearchLongTerm(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]model.AgentMemoryLongTerm, error)
}

type memoryRepository struct {
	pool *pgxpool.Pool
}

func NewMemoryRepository(pool *pgxpool.Pool) MemoryRepository {
	return &memoryRepository{pool: pool}
}

func (r *memoryRepository) UpsertShortTerm(ctx context.Context, mem *model.AgentMemoryShortTerm) error {
	const sql = `
		INSERT INTO agent_memory_short_term (id, agent_id, session_id, messages, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (agent_id, session_id) DO UPDATE
		SET messages = EXCLUDED.messages, updated_at = NOW()`

	if mem.ID == uuid.Nil {
		mem.ID = uuid.New()
	}
	_, err := r.pool.Exec(ctx, sql, mem.ID, mem.AgentID, mem.SessionID, mem.Messages)
	if err != nil {
		return fmt.Errorf("memory_repo: upsert_short_term: %w", err)
	}
	return nil
}

func (r *memoryRepository) GetShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) (*model.AgentMemoryShortTerm, error) {
	const sql = `
		SELECT id, agent_id, session_id, messages, created_at, updated_at
		FROM agent_memory_short_term
		WHERE agent_id = $1 AND session_id = $2`

	var m model.AgentMemoryShortTerm
	err := r.pool.QueryRow(ctx, sql, agentID, sessionID).Scan(
		&m.ID, &m.AgentID, &m.SessionID, &m.Messages, &m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("memory_repo: get_short_term: %w", err)
	}
	return &m, nil
}

func (r *memoryRepository) DeleteShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) error {
	const sql = `DELETE FROM agent_memory_short_term WHERE agent_id = $1 AND session_id = $2`
	_, err := r.pool.Exec(ctx, sql, agentID, sessionID)
	if err != nil {
		return fmt.Errorf("memory_repo: delete_short_term: %w", err)
	}
	return nil
}

func (r *memoryRepository) AppendLongTerm(ctx context.Context, mem *model.AgentMemoryLongTerm) error {
	const sql = `
		INSERT INTO agent_memory_long_term (id, agent_id, org_id, content, summary, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)`

	if mem.ID == uuid.Nil {
		mem.ID = uuid.New()
	}
	metadata := mem.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}
	_, err := r.pool.Exec(ctx, sql, mem.ID, mem.AgentID, mem.OrgID, mem.Content, mem.Summary, metadata)
	if err != nil {
		return fmt.Errorf("memory_repo: append_long_term: %w", err)
	}
	return nil
}

func (r *memoryRepository) SearchLongTerm(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]model.AgentMemoryLongTerm, error) {
	if limit <= 0 {
		limit = 10
	}
	// Text-based search using ILIKE; upgradeable to pgvector cosine similarity.
	const sql = `
		SELECT id, agent_id, org_id, content, summary, metadata, created_at
		FROM agent_memory_long_term
		WHERE agent_id = $1 AND (content ILIKE '%' || $2 || '%' OR summary ILIKE '%' || $2 || '%')
		ORDER BY created_at DESC
		LIMIT $3`

	rows, err := r.pool.Query(ctx, sql, agentID, query, limit)
	if err != nil {
		return nil, fmt.Errorf("memory_repo: search_long_term: %w", err)
	}
	defer rows.Close()

	var results []model.AgentMemoryLongTerm
	for rows.Next() {
		var m model.AgentMemoryLongTerm
		if err := rows.Scan(&m.ID, &m.AgentID, &m.OrgID, &m.Content, &m.Summary, &m.Metadata, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("memory_repo: search_long_term scan: %w", err)
		}
		results = append(results, m)
	}
	return results, nil
}
