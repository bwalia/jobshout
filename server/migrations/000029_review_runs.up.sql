-- Migration 029: PR review runs (in-cluster review-bot sidecar).
--
-- One row per review JobShout queues. The sidecar holds the in-memory OpenCode
-- job; this table is the system of record so an API pod restart does not lose
-- the run. Findings live in result JSONB (the sidecar's review_to_dict).
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot.

CREATE TABLE IF NOT EXISTS review_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    requested_by UUID,
    repo TEXT NOT NULL,
    pr_number INT NOT NULL,
    dry_run BOOLEAN NOT NULL DEFAULT TRUE,
    force BOOLEAN NOT NULL DEFAULT FALSE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed')),
    remote_job_id TEXT,
    head_sha TEXT,
    decision TEXT,
    verdict TEXT,
    summary TEXT,
    github_url TEXT,
    result JSONB,
    stage_log JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message TEXT,
    poll_attempts INT NOT NULL DEFAULT 0,
    next_poll_at TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_review_runs_org_id ON review_runs (org_id);
CREATE INDEX IF NOT EXISTS idx_review_runs_status ON review_runs (status);
CREATE INDEX IF NOT EXISTS idx_review_runs_created_at ON review_runs (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_review_runs_pending ON review_runs (next_poll_at)
    WHERE status IN ('queued', 'running');
