-- Migration 028: heartbeat on blog_runs so a reconciler can tell a live
-- generation apart from one whose process died.
--
-- Generation is an in-process goroutine. SIGTERM cancels it; SIGKILL / OOM /
-- a node drain does not, and the row stays running. The writer touches
-- heartbeat_at every ~30s; a reconciler fails rows whose heartbeat is older
-- than BLOG_ORPHAN_TIMEOUT.
--
-- IDEMPOTENT: migrate.go replays every *.up.sql on every boot.

ALTER TABLE blog_runs
    ADD COLUMN IF NOT EXISTS heartbeat_at TIMESTAMPTZ;

UPDATE blog_runs
SET heartbeat_at = COALESCE(started_at, created_at)
WHERE heartbeat_at IS NULL
  AND status IN ('running', 'pending');

CREATE INDEX IF NOT EXISTS idx_blog_runs_status_heartbeat
    ON blog_runs (status, heartbeat_at)
    WHERE status IN ('running', 'pending');
