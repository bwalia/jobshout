DROP INDEX IF EXISTS idx_course_runs_resuming;
UPDATE course_runs SET status = 'failed' WHERE status = 'resuming';
ALTER TABLE course_runs DROP CONSTRAINT IF EXISTS course_runs_status_check;
ALTER TABLE course_runs ADD CONSTRAINT course_runs_status_check
    CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled'));
ALTER TABLE course_runs DROP COLUMN IF EXISTS attempt;
ALTER TABLE course_runs DROP COLUMN IF EXISTS progress;
ALTER TABLE course_runs DROP COLUMN IF EXISTS research_notes;
