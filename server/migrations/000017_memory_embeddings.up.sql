-- Migration 017: semantic long-term agent memory. Adds a pgvector embedding
-- column to agent_memory_long_term plus a cosine-distance ANN index so that
-- SearchLongTerm can rank recalls by semantic similarity instead of ILIKE
-- substring matching. Rows written before backfill keep a NULL embedding and
-- are transparently served by the ILIKE fallback path in the repository.

CREATE EXTENSION IF NOT EXISTS vector;

ALTER TABLE agent_memory_long_term ADD COLUMN IF NOT EXISTS embedding vector(1536);

-- Approximate-nearest-neighbour index for cosine distance searches.
CREATE INDEX IF NOT EXISTS idx_agent_memory_lt_embedding
    ON agent_memory_long_term USING ivfflat (embedding vector_cosine_ops)
    WITH (lists = 100);
