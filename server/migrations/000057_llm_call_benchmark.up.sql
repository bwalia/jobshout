-- One usage_records row per text LLM call, written by internal/llmbench for
-- the LLM benchmark. The table already had the per-call shape (provider,
-- model, tokens, latency, retries, is_error) but no writer; this adds what a
-- benchmark needs to attribute a call to a run and a step.
--
-- Rows are raw and never averaged: averages, p50/p95, min and max are
-- computed at read time. Prompts, replies and credentials are never stored.
--
-- IDEMPOTENCY IS MANDATORY: this file replays on every boot.

ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS run_kind        VARCHAR(50)  NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS run_id          VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS stage           VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS attempt         INTEGER      NOT NULL DEFAULT 1;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS status          VARCHAR(20)  NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS error           TEXT         NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS requested_model VARCHAR(200) NOT NULL DEFAULT '';
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS started_at      TIMESTAMPTZ;
ALTER TABLE usage_records ADD COLUMN IF NOT EXISTS completed_at    TIMESTAMPTZ;

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
