DROP INDEX IF EXISTS idx_review_runs_task_id;
ALTER TABLE review_runs DROP COLUMN IF EXISTS task_id;
