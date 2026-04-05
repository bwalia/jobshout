-- Rollback migration 003
ALTER TABLE tasks DROP COLUMN IF EXISTS depth;
ALTER TABLE tasks DROP COLUMN IF EXISTS tags;
DROP TABLE IF EXISTS task_dependencies;
DROP TABLE IF EXISTS session_snapshots;
DROP TABLE IF EXISTS sessions;
DROP TABLE IF EXISTS scheduled_task_runs;
DROP TABLE IF EXISTS scheduled_tasks;
DROP TABLE IF EXISTS llm_provider_configs;
