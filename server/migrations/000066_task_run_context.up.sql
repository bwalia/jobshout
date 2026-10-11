-- Migration 066: role context on task runs.
--
-- run_context = {"role": "...", "user_ref": "...", "workspace_ref": "..."}
-- supplied by the caller (e.g. OpsAPI) on POST /tasks/{id}/run or
-- /tasks/launch, so one agent can serve several roles. NULL = none.
ALTER TABLE task_runs ADD COLUMN IF NOT EXISTS run_context JSONB;
