-- Reverse migration 035.
--
-- mail_threads.research_brief_id is left alone: migration 035 did not change
-- its shape, only what mail_service stores in it.
DROP INDEX IF EXISTS research_runs_task_idx;
DROP INDEX IF EXISTS research_runs_agent_created_idx;
DROP INDEX IF EXISTS research_runs_org_created_idx;
DROP TABLE IF EXISTS research_runs;
