-- Reverse migration 036. The specialist tables are untouched: agent_runs names
-- their rows, it does not own them, so dropping it loses the envelope and no
-- actual work.
DROP INDEX IF EXISTS agent_runs_task_idx;
DROP INDEX IF EXISTS agent_runs_org_created_idx;
DROP INDEX IF EXISTS agent_runs_agent_created_idx;
DROP TABLE IF EXISTS agent_runs;
