-- Phase 3: Linked Hire (orgs, requisitions, pipelines). Flag HIRE_ENABLED.

CREATE TABLE IF NOT EXISTS hire_organisations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    slug            TEXT NOT NULL UNIQUE,
    owner_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS hire_org_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES hire_organisations(id) ON DELETE CASCADE,
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role            TEXT NOT NULL DEFAULT 'recruiter'
        CHECK (role IN ('owner', 'admin', 'recruiter', 'hiring_manager', 'interviewer', 'viewer')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, account_id)
);
CREATE INDEX IF NOT EXISTS hire_org_members_account_idx ON hire_org_members (account_id);

CREATE TABLE IF NOT EXISTS hire_requisitions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES hire_organisations(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'permanent'
        CHECK (kind IN ('permanent', 'contract', 'freelance')),
    status          TEXT NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'open', 'on_hold', 'filled', 'cancelled')),
    location        TEXT NOT NULL DEFAULT '',
    remote_ok       BOOLEAN NOT NULL DEFAULT TRUE,
    skills          TEXT[] NOT NULL DEFAULT '{}',
    published_job_id UUID,
    published_project_id UUID,
    created_by      UUID REFERENCES accounts(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS hire_requisitions_org_idx
    ON hire_requisitions (org_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS hire_pipeline_stages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES hire_organisations(id) ON DELETE CASCADE,
    requisition_id  UUID REFERENCES hire_requisitions(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    position        INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS hire_shortlist (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES hire_organisations(id) ON DELETE CASCADE,
    requisition_id  UUID NOT NULL REFERENCES hire_requisitions(id) ON DELETE CASCADE,
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    stage_id        UUID REFERENCES hire_pipeline_stages(id) ON DELETE SET NULL,
    notes_private   TEXT NOT NULL DEFAULT '',
    created_by      UUID REFERENCES accounts(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (requisition_id, member_id)
);
CREATE INDEX IF NOT EXISTS hire_shortlist_org_idx ON hire_shortlist (org_id, requisition_id);

CREATE TABLE IF NOT EXISTS hire_invitations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES hire_organisations(id) ON DELETE CASCADE,
    requisition_id  UUID NOT NULL REFERENCES hire_requisitions(id) ON DELETE CASCADE,
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'expired')),
    message         TEXT NOT NULL DEFAULT '',
    -- Contact details revealed only after accept (stored encrypted later; empty for now).
    contact_revealed_at TIMESTAMPTZ,
    created_by      UUID REFERENCES accounts(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS hire_invitations_member_idx
    ON hire_invitations (member_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS hire_saved_searches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES hire_organisations(id) ON DELETE CASCADE,
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    query           JSONB NOT NULL DEFAULT '{}'::jsonb,
    alert_enabled   BOOLEAN NOT NULL DEFAULT FALSE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
