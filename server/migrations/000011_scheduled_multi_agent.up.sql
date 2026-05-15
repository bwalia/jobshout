-- Phase 1 of the JobShout "scheduled multi-agent" feature.
--
-- Lets a customer schedule a planner→executor→reviewer collaboration with the
-- same cadence options (cron / interval / once) as existing scheduled_tasks,
-- plus friendly presets like "every_midnight" or "every_morning_10am" that are
-- translated to cron at create-time by the handler.

ALTER TABLE scheduled_tasks
    ADD COLUMN IF NOT EXISTS planner_id        UUID REFERENCES agents(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS executor_id       UUID REFERENCES agents(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS reviewer_id       UUID REFERENCES agents(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS max_review        INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN IF NOT EXISTS schedule_preset   VARCHAR(50);

-- Allow scheduled_tasks.task_type to be 'multi_agent' going forward.
-- (No CHECK constraint exists today; the handler validates the value.)

ALTER TABLE scheduled_task_runs
    ADD COLUMN IF NOT EXISTS multi_agent_job_id UUID REFERENCES multi_agent_jobs(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_ma_job
    ON scheduled_task_runs(multi_agent_job_id)
    WHERE multi_agent_job_id IS NOT NULL;
