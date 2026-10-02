-- Migration 059: resumable Course Generator runs.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

-- The notes every later stage is written from. Saved when research finishes so
-- a resumed run does not research again (and plan a different course).
ALTER TABLE course_runs ADD COLUMN IF NOT EXISTS research_notes TEXT;

-- What is done beyond the saved outline and chapters: the cover, and the
-- chapter in flight with a flag per finished stage.
ALTER TABLE course_runs ADD COLUMN IF NOT EXISTS progress JSONB;

-- Each resume is a new attempt. An attempt guards every write with its own
-- number, so a goroutine left over from an earlier attempt (on this pod or
-- another replica) cannot write onto the run's current one.
ALTER TABLE course_runs ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1;

-- 'resuming': the server that was generating went away; a server will pick the
-- run up from its saved state.
ALTER TABLE course_runs DROP CONSTRAINT IF EXISTS course_runs_status_check;
ALTER TABLE course_runs ADD CONSTRAINT course_runs_status_check
    CHECK (status IN ('queued', 'running', 'resuming', 'completed', 'failed', 'cancelled'));

CREATE INDEX IF NOT EXISTS idx_course_runs_resuming ON course_runs(status) WHERE status = 'resuming';
