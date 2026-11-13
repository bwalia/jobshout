-- Phase 5a: AI drafts from Go platform POST /tasks/launch (human must approve).

CREATE TABLE IF NOT EXISTS ai_drafts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL
        CHECK (kind IN ('proposal', 'project_breakdown', 'talent_match')),
    status          TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'discarded', 'applied')),
    title           TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL DEFAULT '',
    input           JSONB NOT NULL DEFAULT '{}'::jsonb,
    platform_task_id TEXT NOT NULL DEFAULT '',
    platform_run_id  TEXT NOT NULL DEFAULT '',
    reference_type  TEXT NOT NULL DEFAULT '',
    reference_id    TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS ai_drafts_account_idx
    ON ai_drafts (account_id, status, created_at DESC);
