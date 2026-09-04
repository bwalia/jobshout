-- JobShout.com Phase 1 schema
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS organisations (
    id          UUID PRIMARY KEY,
    name        TEXT NOT NULL,
    slug        TEXT NOT NULL UNIQUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS jobs (
    id               UUID PRIMARY KEY,
    organisation_id  UUID NOT NULL REFERENCES organisations(id),
    title            TEXT NOT NULL,
    summary          TEXT NOT NULL DEFAULT '',
    description      TEXT NOT NULL,
    employment_type  TEXT NOT NULL,
    location         JSONB NOT NULL DEFAULT '{}'::jsonb,
    compensation     JSONB NOT NULL DEFAULT '{}'::jsonb,
    requirements     JSONB NOT NULL DEFAULT '[]'::jsonb,
    status           TEXT NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at     TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS jobs_status_published_idx
    ON jobs (status, published_at DESC NULLS LAST);

CREATE INDEX IF NOT EXISTS jobs_org_idx ON jobs (organisation_id);
