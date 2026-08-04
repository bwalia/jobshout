-- Migration 016: pgvector-backed knowledge chunks for retrieval-augmented
-- generation (RAG). Each knowledge file is split into overlapping chunks that
-- are embedded and stored for cosine-similarity search.

CREATE EXTENSION IF NOT EXISTS vector;

CREATE TABLE IF NOT EXISTS knowledge_chunks (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id             UUID NOT NULL,
    agent_id           UUID NOT NULL,
    knowledge_file_id  UUID NOT NULL REFERENCES knowledge_files(id) ON DELETE CASCADE,
    chunk_index        INT  NOT NULL,
    content            TEXT NOT NULL,
    embedding          vector(1536),
    metadata           JSONB DEFAULT '{}',
    created_at         TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_agent ON knowledge_chunks(agent_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_file  ON knowledge_chunks(knowledge_file_id);

-- Approximate-nearest-neighbour index for cosine distance searches.
CREATE INDEX IF NOT EXISTS idx_knowledge_chunks_embedding
    ON knowledge_chunks USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
