DROP INDEX IF EXISTS idx_blog_runs_status_heartbeat;
ALTER TABLE blog_runs DROP COLUMN IF EXISTS heartbeat_at;
