-- Candidate profiles for AI job matching
CREATE TABLE IF NOT EXISTS candidate_profiles (
    id                         UUID PRIMARY KEY,
    email                      TEXT NOT NULL UNIQUE,
    display_name               TEXT NOT NULL,
    headline                   TEXT NOT NULL DEFAULT '',
    summary                    TEXT NOT NULL DEFAULT '',
    skills                     JSONB NOT NULL DEFAULT '[]'::jsonb,
    years_experience           INT,
    preferred_roles            JSONB NOT NULL DEFAULT '[]'::jsonb,
    preferred_locations        JSONB NOT NULL DEFAULT '[]'::jsonb,
    preferred_employment_types JSONB NOT NULL DEFAULT '[]'::jsonb,
    open_to_remote             BOOLEAN NOT NULL DEFAULT TRUE,
    salary_expectation         JSONB NOT NULL DEFAULT '{}'::jsonb,
    cv_text                    TEXT NOT NULL DEFAULT '',
    matching_notes             TEXT NOT NULL DEFAULT '',
    created_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at                 TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS candidate_profiles_email_idx
    ON candidate_profiles (lower(email));
