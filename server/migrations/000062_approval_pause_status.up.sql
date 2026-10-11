-- Migration 062: record runs paused for human approval as such.
--
-- A run that paused on a gated tool used to be saved as `completed` with an
-- empty output, so callers polling it took the pause for a finished run. It is
-- now `awaiting_approval` on both the execution and the task run, the task run
-- carries the pending approval's id, and a decided approval records when its
-- resume finished (resumed_at) and when a worker claimed it
-- (resume_claimed_at), so a resume lost to a restart is picked up again.
--
-- IDEMPOTENT: database/migrate.go replays every *.up.sql on every boot.
-- agent_executions.status has no CHECK constraint, so only task_runs needs one
-- widened.

DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'task_runs'::regclass AND conname = 'task_runs_status_check'
          AND pg_get_constraintdef(oid) NOT LIKE '%awaiting_approval%'
    ) THEN
        ALTER TABLE task_runs DROP CONSTRAINT task_runs_status_check;
    END IF;
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint
        WHERE conrelid = 'task_runs'::regclass AND conname = 'task_runs_status_check'
    ) THEN
        ALTER TABLE task_runs ADD CONSTRAINT task_runs_status_check
            CHECK (status IN ('queued','running','awaiting_approval','completed','failed'));
    END IF;
END $$;

ALTER TABLE task_runs ADD COLUMN IF NOT EXISTS pending_approval_id UUID;

ALTER TABLE approvals ADD COLUMN IF NOT EXISTS resume_claimed_at TIMESTAMPTZ;

-- The one-off backfill runs only on the boot that adds resumed_at, never on a
-- replay: afterwards a decided approval with resumed_at NULL is a genuinely
-- unfinished resume that the server must retry.
DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = current_schema() AND table_name = 'approvals' AND column_name = 'resumed_at'
    ) THEN
        ALTER TABLE approvals ADD COLUMN resumed_at TIMESTAMPTZ;

        -- Approvals decided before this migration were resumed in the request.
        UPDATE approvals SET resumed_at = COALESCE(decided_at, NOW())
        WHERE status <> 'pending';

        -- Runs paused under the old code were saved as completed; mark the
        -- ones still waiting on a human for what they are.
        UPDATE agent_executions e SET status = 'awaiting_approval', completed_at = NULL
        WHERE e.status = 'completed'
          AND EXISTS (SELECT 1 FROM approvals a WHERE a.execution_id = e.id AND a.status = 'pending');

        UPDATE task_runs r SET status = 'awaiting_approval', completed_at = NULL,
               pending_approval_id = (
                   SELECT a.id FROM approvals a
                   WHERE a.execution_id = r.execution_id AND a.status = 'pending'
                   ORDER BY a.requested_at DESC LIMIT 1)
        WHERE r.status = 'completed'
          AND EXISTS (SELECT 1 FROM approvals a WHERE a.execution_id = r.execution_id AND a.status = 'pending');
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_approvals_unresumed ON approvals(decided_at)
    WHERE status <> 'pending' AND resumed_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_task_runs_execution ON task_runs(execution_id);
