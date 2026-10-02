-- One usage_records row per text LLM call, written by internal/llmbench for
-- LLM usage tracking and the benchmark. The table already had the per-call
-- shape (provider, model, tokens, latency, retries, is_error, agent_id,
-- execution_id, task_id); this adds what is needed to attribute a call to a
-- run, a task run and a step, and to record what the provider itself reported.
--
-- Rows are raw and never averaged: averages, p50/p95, min and max are
-- computed at read time. Prompts, replies and credentials are never stored.
-- Rows written here always have a non-empty status; that is how they are told
-- apart from any per-execution summary row.
--
-- Nothing is guessed: a value the provider did not report is NULL.
--   model            the model the provider's reply named (model_reported),
--                    else the model that was sent
--   requested_model  the model sent: the caller's, or the client's default
--   latency_ms       the whole call, retries and backoff included
--   api_duration_ms  only the HTTP exchanges with the provider
--   api_attempts     HTTP requests actually sent (1 + transport retries);
--                    metadata.api_attempts lists each one
--
-- One row is one logical Generate call; its individual HTTP attempts are
-- api_attempts / metadata.api_attempts. Failed calls are rows too
-- (status 'failed' or 'cancelled').
--
-- IDEMPOTENCY IS MANDATORY: this file replays on every boot.

ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS run_kind            VARCHAR(50)  NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS run_id              VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS task_run_id         UUID;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS stage               VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS attempt             INTEGER      NOT NULL DEFAULT 1;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS status              VARCHAR(20)  NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS error               TEXT         NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS requested_model     VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS model_reported      BOOLEAN      NOT NULL DEFAULT false;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS provider_request_id VARCHAR(200);
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS tokens_total        INTEGER;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS tokens_reasoning    INTEGER;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS api_duration_ms     INTEGER;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS api_attempts        INTEGER;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS started_at          TIMESTAMPTZ;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS completed_at        TIMESTAMPTZ;

-- A provider-reported model name (e.g. a dated Gemini version) can exceed 100.
-- Widening a varchar needs no rewrite, but ALTER TYPE still takes an exclusive
-- lock, so it runs only while the column is narrower — not on every boot.
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND table_name = 'usage_records'
          AND column_name = 'model' AND character_maximum_length < 200
    ) THEN
        ALTER TABLE usage_records ALTER COLUMN model TYPE VARCHAR(200);
    END IF;
END $$;

-- A token count the provider did not report is unknown, not zero.
ALTER TABLE usage_records ALTER COLUMN tokens_in  DROP NOT NULL;
ALTER TABLE usage_records ALTER COLUMN tokens_in  DROP DEFAULT;
ALTER TABLE usage_records ALTER COLUMN tokens_out DROP NOT NULL;
ALTER TABLE usage_records ALTER COLUMN tokens_out DROP DEFAULT;

-- Calls made outside any org (e.g. intent routing) are still recorded; the
-- org-scoped API simply never returns them.
ALTER TABLE usage_records ALTER COLUMN org_id DROP NOT NULL;

CREATE INDEX IF NOT EXISTS idx_usage_records_org_run
    ON usage_records(org_id, run_kind, run_id);
CREATE INDEX IF NOT EXISTS idx_usage_records_org_model_created
    ON usage_records(org_id, provider, model, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_records_task_run
    ON usage_records(task_run_id) WHERE task_run_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_usage_records_task
    ON usage_records(task_id, created_at) WHERE task_id IS NOT NULL;

-- Lets a resumed execution's calls find their task run (see the recorder's
-- insert), and a task run's calls be found by execution.
CREATE INDEX IF NOT EXISTS idx_task_runs_execution
    ON task_runs(execution_id) WHERE execution_id IS NOT NULL;
