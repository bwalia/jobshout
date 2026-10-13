-- Migration 068: Video Producer runs.
--
-- One row per launch: what was asked for, the script, and where the render is
-- (writing the script, waiting for the workstation, narration, keyframes,
-- animation, editing, saving). The Video Producer tab reads it to show what
-- the agent is doing; the service writes it as the run moves. The id is also
-- the workstation's job id.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

CREATE TABLE IF NOT EXISTS video_runs (
    id               UUID PRIMARY KEY,
    org_id           UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    task_id          UUID,
    agent_id         UUID,
    requested_by     UUID,
    topic            TEXT NOT NULL DEFAULT '',
    brief            JSONB NOT NULL DEFAULT '{}'::jsonb,
    title            TEXT NOT NULL DEFAULT '',
    shots            JSONB NOT NULL DEFAULT '[]'::jsonb,
    status           VARCHAR(32) NOT NULL DEFAULT 'scripting',
    stage            VARCHAR(32) NOT NULL DEFAULT 'script',
    progress         REAL NOT NULL DEFAULT 0,
    message          TEXT NOT NULL DEFAULT '',
    error_message    TEXT NOT NULL DEFAULT '',
    notes            JSONB NOT NULL DEFAULT '[]'::jsonb,
    video_url        TEXT NOT NULL DEFAULT '',
    duration_seconds REAL NOT NULL DEFAULT 0,
    motion_model     TEXT NOT NULL DEFAULT '',
    estimate_minutes INTEGER NOT NULL DEFAULT 0,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_video_runs_org_created
    ON video_runs (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_video_runs_unfinished
    ON video_runs (status) WHERE status IN ('scripting', 'queued', 'rendering');
CREATE INDEX IF NOT EXISTS idx_video_runs_task
    ON video_runs (task_id) WHERE task_id IS NOT NULL;
