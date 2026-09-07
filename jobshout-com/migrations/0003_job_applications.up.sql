-- Job applications: candidates apply to published jobs.
CREATE TABLE IF NOT EXISTS job_applications (
    id                    UUID PRIMARY KEY,
    job_id                UUID NOT NULL REFERENCES jobs(id) ON DELETE CASCADE,
    candidate_profile_id  UUID REFERENCES candidate_profiles(id) ON DELETE SET NULL,
    full_name             TEXT NOT NULL,
    email                 TEXT NOT NULL,
    phone                 TEXT NOT NULL DEFAULT '',
    cover_letter          TEXT NOT NULL DEFAULT '',
    cv_text               TEXT NOT NULL DEFAULT '',
    cv_url                TEXT NOT NULL DEFAULT '',
    status                TEXT NOT NULL DEFAULT 'submitted',
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One application per person per role; re-applying updates the existing row.
CREATE UNIQUE INDEX IF NOT EXISTS job_applications_job_email_idx
    ON job_applications (job_id, lower(email));

CREATE INDEX IF NOT EXISTS job_applications_email_idx
    ON job_applications (lower(email), created_at DESC);

CREATE INDEX IF NOT EXISTS job_applications_job_idx
    ON job_applications (job_id, created_at DESC);
