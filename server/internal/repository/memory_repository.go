package repository

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jobshout/server/internal/model"
)

// Embedder is the narrow interface the memory repository needs to turn text
// into dense vectors for semantic recall. It is structurally satisfied by
// llm.Embedder, so callers can inject the shared embedder without this package
// importing the llm package.
type Embedder interface {
	// Embed returns one embedding vector per input text, in the same order.
	Embed(ctx context.Context, texts []string) ([][]float32, error)
}

// MemoryRepository manages agent short-term and long-term memory persistence.
type MemoryRepository interface {
	UpsertShortTerm(ctx context.Context, mem *model.AgentMemoryShortTerm) error
	GetShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) (*model.AgentMemoryShortTerm, error)
	DeleteShortTerm(ctx context.Context, agentID, sessionID uuid.UUID) error

	AppendLongTerm(ctx context.Context, mem *model.AgentMemoryLongTerm) error
	SearchLongTerm(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]model.AgentMemoryLongTerm, error)

	// WithEmbedder attaches an embedder enabling semantic long-term recall and
	// embedding-on-write. It mutates the receiver and returns it for chaining.
	// Passing a nil embedder leaves the repository on the ILIKE fallback path.
	WithEmbedder(e Embedder) MemoryRepository
}

type memoryRepository struct {
	pool     *pgxpool.Pool
	embedder Embedder // nil when embeddings are not configured
}

func NewMemoryRepository(pool *pgxpool.Pool) MemoryRepository {
	return &memoryRepository{pool: pool}
}

func (r *memoryRepository) WithEmbedder(e Embedder) MemoryRepository {
	r.embedder = e
	return r
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

// appendLongTermSQL stores a long-term memory. The embedding is cast from text
// so a NULL argument yields a NULL vector (pre-backfill / no embedder), which
// the ILIKE fallback path serves transparently.
const appendLongTermSQL = `
	INSERT INTO agent_memory_long_term (id, agent_id, org_id, content, summary, metadata, embedding)
	VALUES ($1, $2, $3, $4, $5, $6, $7::vector)`

func (r *memoryRepository) AppendLongTerm(ctx context.Context, mem *model.AgentMemoryLongTerm) error {
	if mem.ID == uuid.Nil {
		mem.ID = uuid.New()
	}
	metadata := mem.Metadata
	if metadata == nil {
		metadata = map[string]any{}
	}

	// Embed on write when an embedder is configured and the caller has not
	// already supplied a vector. Failures degrade gracefully to a NULL
	// embedding rather than blocking the write.
	if len(mem.Embedding) == 0 {
		if emb := r.embedText(ctx, memoryEmbedInput(mem)); emb != nil {
			mem.Embedding = emb
		}
	}

	var embeddingArg any // nil => NULL::vector
	if len(mem.Embedding) > 0 {
		embeddingArg = VectorString(mem.Embedding)
	}

	_, err := r.pool.Exec(ctx, appendLongTermSQL,
		mem.ID, mem.AgentID, mem.OrgID, mem.Content, mem.Summary, metadata, embeddingArg)
	if err != nil {
		return fmt.Errorf("memory_repo: append_long_term: %w", err)
	}
	return nil
}

// longTermVectorSearchSQL ranks a memory by cosine distance to the query
// embedding. Only embedded rows participate; pre-backfill rows (NULL embedding)
// are excluded here and remain reachable via the ILIKE fallback.
const longTermVectorSearchSQL = `
	SELECT id, agent_id, org_id, content, summary, metadata, created_at,
	       (embedding <=> $2::vector) AS distance
	FROM agent_memory_long_term
	WHERE agent_id = $1 AND embedding IS NOT NULL
	ORDER BY embedding <=> $2::vector
	LIMIT $3`

// longTermTextSearchSQL is the ILIKE substring fallback used when no embedder
// is configured or the query cannot be embedded.
const longTermTextSearchSQL = `
	SELECT id, agent_id, org_id, content, summary, metadata, created_at
	FROM agent_memory_long_term
	WHERE agent_id = $1 AND (content ILIKE '%' || $2 || '%' OR summary ILIKE '%' || $2 || '%')
	ORDER BY created_at DESC
	LIMIT $3`

func (r *memoryRepository) SearchLongTerm(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]model.AgentMemoryLongTerm, error) {
	if limit <= 0 {
		limit = 10
	}

	// Semantic path: embed the query and order by cosine distance. Falls back
	// to ILIKE when no embedder is configured or embedding fails, so recall
	// keeps working before the embedding backfill has run.
	if emb := r.embedText(ctx, query); emb != nil {
		return r.searchLongTermByVector(ctx, agentID, emb, limit)
	}
	return r.searchLongTermByText(ctx, agentID, query, limit)
}

func (r *memoryRepository) searchLongTermByVector(ctx context.Context, agentID uuid.UUID, embedding []float32, limit int) ([]model.AgentMemoryLongTerm, error) {
	rows, err := r.pool.Query(ctx, longTermVectorSearchSQL, agentID, VectorString(embedding), limit)
	if err != nil {
		return nil, fmt.Errorf("memory_repo: search_long_term vector: %w", err)
	}
	defer rows.Close()

	var results []model.AgentMemoryLongTerm
	for rows.Next() {
		var m model.AgentMemoryLongTerm
		if err := rows.Scan(&m.ID, &m.AgentID, &m.OrgID, &m.Content, &m.Summary, &m.Metadata, &m.CreatedAt, &m.Distance); err != nil {
			return nil, fmt.Errorf("memory_repo: search_long_term vector scan: %w", err)
		}
		results = append(results, m)
	}
	return results, nil
}

func (r *memoryRepository) searchLongTermByText(ctx context.Context, agentID uuid.UUID, query string, limit int) ([]model.AgentMemoryLongTerm, error) {
	rows, err := r.pool.Query(ctx, longTermTextSearchSQL, agentID, query, limit)
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

// embedText returns the embedding of a single text, or nil when no embedder is
// configured, the input is empty, or embedding fails. Returning nil signals the
// caller to take the ILIKE / NULL-embedding fallback path.
func (r *memoryRepository) embedText(ctx context.Context, text string) []float32 {
	if r.embedder == nil || text == "" {
		return nil
	}
	vecs, err := r.embedder.Embed(ctx, []string{text})
	if err != nil || len(vecs) == 0 || len(vecs[0]) == 0 {
		return nil
	}
	return vecs[0]
}

// memoryEmbedInput picks the text used to embed a long-term memory, preferring
// the full content and falling back to the summary.
func memoryEmbedInput(mem *model.AgentMemoryLongTerm) string {
	if mem.Content != "" {
		return mem.Content
	}
	return mem.Summary
}
