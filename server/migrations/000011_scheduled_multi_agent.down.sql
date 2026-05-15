DROP INDEX IF EXISTS idx_scheduled_task_runs_ma_job;

ALTER TABLE scheduled_task_runs
    DROP COLUMN IF EXISTS multi_agent_job_id;

ALTER TABLE scheduled_tasks
    DROP COLUMN IF EXISTS schedule_preset,
    DROP COLUMN IF EXISTS max_review,
    DROP COLUMN IF EXISTS reviewer_id,
    DROP COLUMN IF EXISTS executor_id,
    DROP COLUMN IF EXISTS planner_id;
