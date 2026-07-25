package repository

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jobshout/server/internal/model"
)

// KnowledgeChunkRepository manages persistence of embedded knowledge chunks in
// the pgvector-backed knowledge_chunks table.
type KnowledgeChunkRepository interface {
	// Create batch-inserts the given chunks. It is a no-op for an empty slice.
	Create(ctx context.Context, chunks []model.KnowledgeChunk) error
	// DeleteByFile removes all chunks belonging to a knowledge file.
	DeleteByFile(ctx context.Context, knowledgeFileID uuid.UUID) error
	// SearchByAgent returns the k chunks for an agent whose embeddings are
	// closest to queryEmbedding, ordered by ascending cosine distance.
	SearchByAgent(ctx context.Context, agentID uuid.UUID, queryEmbedding []float32, k int) ([]model.KnowledgeChunk, error)
}

type knowledgeChunkRepository struct {
	pool *pgxpool.Pool
}

// NewKnowledgeChunkRepository creates a new KnowledgeChunkRepository.
func NewKnowledgeChunkRepository(pool *pgxpool.Pool) KnowledgeChunkRepository {
	return &knowledgeChunkRepository{pool: pool}
}

// VectorString renders a float32 slice in pgvector's text input format, e.g.
// "[0.1,0.2,0.3]". It is exported so callers (and tests) can build query
// vectors without importing pgvector's own types.
func VectorString(v []float32) string {
	if len(v) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(f), 'f', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}

func (r *knowledgeChunkRepository) Create(ctx context.Context, chunks []model.KnowledgeChunk) error {
	if len(chunks) == 0 {
		return nil
	}

	const sql = `
		INSERT INTO knowledge_chunks
			(org_id, agent_id, knowledge_file_id, chunk_index, content, embedding, metadata)
		VALUES ($1, $2, $3, $4, $5, $6::vector, $7)`

	batch := &pgx.Batch{}
	for _, c := range chunks {
		metadata := c.Metadata
		if metadata == nil {
			metadata = map[string]any{}
		}
		batch.Queue(sql, c.OrgID, c.AgentID, c.KnowledgeFileID, c.ChunkIndex, c.Content, VectorString(c.Embedding), metadata)
	}

	br := r.pool.SendBatch(ctx, batch)
	defer br.Close()

	for range chunks {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("knowledge_chunk_repo: create: %w", err)
		}
	}
	return nil
}

func (r *knowledgeChunkRepository) DeleteByFile(ctx context.Context, knowledgeFileID uuid.UUID) error {
	const sql = `DELETE FROM knowledge_chunks WHERE knowledge_file_id = $1`
	if _, err := r.pool.Exec(ctx, sql, knowledgeFileID); err != nil {
		return fmt.Errorf("knowledge_chunk_repo: delete_by_file: %w", err)
	}
	return nil
}

func (r *knowledgeChunkRepository) SearchByAgent(ctx context.Context, agentID uuid.UUID, queryEmbedding []float32, k int) ([]model.KnowledgeChunk, error) {
	if k <= 0 {
		k = 5
	}

	const sql = `
		SELECT id, org_id, agent_id, knowledge_file_id, chunk_index, content, metadata, created_at,
		       (embedding <=> $2::vector) AS distance
		FROM knowledge_chunks
		WHERE agent_id = $1 AND embedding IS NOT NULL
		ORDER BY embedding <=> $2::vector
		LIMIT $3`

	rows, err := r.pool.Query(ctx, sql, agentID, VectorString(queryEmbedding), k)
	if err != nil {
		return nil, fmt.Errorf("knowledge_chunk_repo: search_by_agent: %w", err)
	}
	defer rows.Close()

	var results []model.KnowledgeChunk
	for rows.Next() {
		var c model.KnowledgeChunk
		if err := rows.Scan(
			&c.ID, &c.OrgID, &c.AgentID, &c.KnowledgeFileID, &c.ChunkIndex,
			&c.Content, &c.Metadata, &c.CreatedAt, &c.Distance,
		); err != nil {
			return nil, fmt.Errorf("knowledge_chunk_repo: search_by_agent scan: %w", err)
		}
		results = append(results, c)
	}
	return results, nil
}
