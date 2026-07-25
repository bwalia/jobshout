-- Reverse migration 017. The vector extension is intentionally left installed
-- as other objects (e.g. knowledge_chunks) may depend on it.
DROP INDEX IF EXISTS idx_agent_memory_lt_embedding;
ALTER TABLE agent_memory_long_term DROP COLUMN IF EXISTS embedding;
