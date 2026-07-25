package model

import (
	"time"

	"github.com/google/uuid"
)

// KnowledgeChunk is a single embedded slice of a knowledge file, stored in the
// pgvector-backed knowledge_chunks table for retrieval-augmented generation.
type KnowledgeChunk struct {
	ID              uuid.UUID      `json:"id"`
	OrgID           uuid.UUID      `json:"org_id"`
	AgentID         uuid.UUID      `json:"agent_id"`
	KnowledgeFileID uuid.UUID      `json:"knowledge_file_id"`
	ChunkIndex      int            `json:"chunk_index"`
	Content         string         `json:"content"`
	Embedding       []float32      `json:"-"`
	Metadata        map[string]any `json:"metadata"`
	CreatedAt       time.Time      `json:"created_at"`

	// Distance is populated by similarity searches (cosine distance; lower is
	// more similar). It is zero-valued outside of search results.
	Distance float64 `json:"distance,omitempty"`
}
