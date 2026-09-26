DROP TABLE IF EXISTS showcase_job_links;
DROP INDEX IF EXISTS jobs_poster_idx;
ALTER TABLE jobs DROP COLUMN IF EXISTS poster_email;
