DROP INDEX IF EXISTS idx_tasks_completed_at;
ALTER TABLE tasks DROP COLUMN IF EXISTS completed_at;
