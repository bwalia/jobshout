-- Phase 2a: make Projects a live marketplace (still free; credits in 2b).

ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS owner_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS title TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS description TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS summary TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'open',
    ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'public',
    ADD COLUMN IF NOT EXISTS featured_until TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS experience_level TEXT NOT NULL DEFAULT '';

DO $$ BEGIN
    ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_status_chk;
    ALTER TABLE projects ADD CONSTRAINT projects_status_chk
        CHECK (status IN ('draft', 'open', 'in_review', 'awarded', 'closed', 'cancelled'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_visibility_chk;
    ALTER TABLE projects ADD CONSTRAINT projects_visibility_chk
        CHECK (visibility IN ('public', 'invite_only'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS projects_owner_idx ON projects (owner_account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS projects_status_idx ON projects (status, created_at DESC);
CREATE INDEX IF NOT EXISTS projects_open_bidding_idx
    ON projects (bidding_end_at) WHERE status = 'open';

ALTER TABLE bids
    ADD COLUMN IF NOT EXISTS credits_spent INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS viewed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS decline_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE IF NOT EXISTS project_invitations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    invitee_member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    invited_by_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    status          TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'expired')),
    message         TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (project_id, invitee_member_id)
);
CREATE INDEX IF NOT EXISTS project_invitations_invitee_idx
    ON project_invitations (invitee_member_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS engagements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id      UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    bid_id          UUID REFERENCES bids(id) ON DELETE SET NULL,
    client_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    freelancer_id   UUID NOT NULL REFERENCES candidate_profiles(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'completed', 'cancelled')),
    started_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS engagements_project_idx ON engagements (project_id);
CREATE INDEX IF NOT EXISTS engagements_freelancer_idx ON engagements (freelancer_id, status);

CREATE TABLE IF NOT EXISTS engagement_reviews (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    engagement_id   UUID NOT NULL REFERENCES engagements(id) ON DELETE CASCADE,
    author_member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    rating          INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body            TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (engagement_id, author_member_id)
);
