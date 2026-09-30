-- Each retry of a blog run is a new attempt. The writer of an attempt guards
-- every write with its own number, so a goroutine left over from a cancelled
-- or retried attempt (on this pod or another replica) cannot write onto the
-- run's current one.
ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS attempt INTEGER NOT NULL DEFAULT 1;
