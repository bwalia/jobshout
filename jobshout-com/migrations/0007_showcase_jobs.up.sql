-- AI Showcase phase 3: link showcase entries (apps, agents, teams) to open
-- jobs on the board. A creator may only link jobs they posted, so jobs now
-- record who posted them (empty for jobs posted anonymously, as all were
-- before this migration). The poster is never returned by the jobs API.

ALTER TABLE jobs ADD COLUMN IF NOT EXISTS poster_email TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS jobs_poster_idx
    ON jobs (lower(poster_email), published_at DESC) WHERE poster_email <> '';

CREATE TABLE IF NOT EXISTS showcase_job_links (
    entry_id  UUID NOT NULL REFERENCES showcase_apps(id) ON DELETE CASCADE,
    job_id    UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    position  INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (entry_id, job_id)
);

CREATE INDEX IF NOT EXISTS showcase_job_links_job_idx ON showcase_job_links (job_id);
