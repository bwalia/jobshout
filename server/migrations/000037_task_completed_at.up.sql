-- Task completion timestamp for the board and Task Manager.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on
-- every boot — there is no schema_migrations table. ADD COLUMN IF NOT EXISTS
-- and a guarded backfill keep this safe to replay.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS completed_at TIMESTAMPTZ;

-- Best-effort: existing done cards get a completion time so the UI is not blank.
-- Only fill rows that are done and still null, so a later edit of a done task
-- (which bumps updated_at) cannot rewrite a timestamp we already stored.
UPDATE tasks
SET completed_at = updated_at
WHERE status = 'done' AND completed_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_tasks_completed_at ON tasks (completed_at)
    WHERE completed_at IS NOT NULL;
