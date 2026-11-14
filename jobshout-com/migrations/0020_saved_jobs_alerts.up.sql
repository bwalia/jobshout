-- Saved jobs and job alerts (saved searches that notify in-app when a
-- matching job is published). Idempotent: replays on every boot.

CREATE TABLE IF NOT EXISTS saved_jobs (
    account_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    job_id      UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, job_id)
);

CREATE INDEX IF NOT EXISTS saved_jobs_account_idx
    ON saved_jobs (account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS job_alerts (
    id          UUID PRIMARY KEY,
    account_id  UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    q           TEXT NOT NULL DEFAULT '',
    location    TEXT NOT NULL DEFAULT '',
    types       TEXT[] NOT NULL DEFAULT '{}',
    remote      BOOLEAN NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS job_alerts_account_idx ON job_alerts (account_id);

-- One alert per identical search per account.
CREATE UNIQUE INDEX IF NOT EXISTS job_alerts_unique_idx
    ON job_alerts (account_id, lower(q), lower(location), types, remote);
