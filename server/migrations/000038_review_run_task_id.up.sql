-- Link a PR review run back to the Task Manager card that launched it so the
-- reconciler can mark that card done when the sidecar finishes.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot.

ALTER TABLE review_runs ADD COLUMN IF NOT EXISTS task_id UUID REFERENCES tasks(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_review_runs_task_id ON review_runs (task_id)
    WHERE task_id IS NOT NULL;
