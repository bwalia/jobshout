DROP INDEX IF EXISTS idx_task_runs_execution;
DROP INDEX IF EXISTS idx_approvals_unresumed;
ALTER TABLE approvals DROP COLUMN IF EXISTS resumed_at;
ALTER TABLE approvals DROP COLUMN IF EXISTS resume_claimed_at;
ALTER TABLE task_runs DROP COLUMN IF EXISTS pending_approval_id;
UPDATE task_runs SET status = 'running' WHERE status = 'awaiting_approval';
ALTER TABLE task_runs DROP CONSTRAINT IF EXISTS task_runs_status_check;
ALTER TABLE task_runs ADD CONSTRAINT task_runs_status_check
    CHECK (status IN ('queued','running','completed','failed'));
