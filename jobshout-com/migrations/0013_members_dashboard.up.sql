-- Phase 1: rich member profiles, connections, messaging, dashboard prefs.
-- Additive only. Flags (MEMBERS_RICH_ENABLED / DASHBOARD_ENABLED / MESSAGING_ENABLED)
-- gate the product surfaces; tables always exist so flags can flip without migrate.

ALTER TABLE members
    ADD COLUMN IF NOT EXISTS bio TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS avatar_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS location TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS timezone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS availability TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS rate_min NUMERIC(12, 2),
    ADD COLUMN IF NOT EXISTS rate_max NUMERIC(12, 2),
    ADD COLUMN IF NOT EXISTS rate_currency TEXT NOT NULL DEFAULT 'GBP',
    ADD COLUMN IF NOT EXISTS languages TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS links JSONB NOT NULL DEFAULT '[]',
    ADD COLUMN IF NOT EXISTS services_offered TEXT[] NOT NULL DEFAULT '{}',
    ADD COLUMN IF NOT EXISTS hide_from_employer BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS open_to_work BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS open_to_hire BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS connection_count INT NOT NULL DEFAULT 0;

DO $$ BEGIN
    ALTER TABLE members
        ADD CONSTRAINT members_availability_chk
        CHECK (availability IN ('', 'full_time', 'part_time', 'contract', 'not_available'));
EXCEPTION WHEN duplicate_object THEN NULL;
END $$;

CREATE TABLE IF NOT EXISTS member_experience (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    organisation    TEXT NOT NULL DEFAULT '',
    location        TEXT NOT NULL DEFAULT '',
    started_on      DATE,
    ended_on        DATE,
    is_current      BOOLEAN NOT NULL DEFAULT FALSE,
    summary         TEXT NOT NULL DEFAULT '',
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS member_experience_member_idx
    ON member_experience (member_id, sort_order, started_on DESC NULLS LAST);

CREATE TABLE IF NOT EXISTS member_education (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    school          TEXT NOT NULL,
    degree          TEXT NOT NULL DEFAULT '',
    field_of_study  TEXT NOT NULL DEFAULT '',
    started_on      DATE,
    ended_on        DATE,
    summary         TEXT NOT NULL DEFAULT '',
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS member_education_member_idx
    ON member_education (member_id, sort_order);

CREATE TABLE IF NOT EXISTS member_certifications (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    name            TEXT NOT NULL,
    issuer          TEXT NOT NULL DEFAULT '',
    issued_on       DATE,
    expires_on      DATE,
    credential_url  TEXT NOT NULL DEFAULT '',
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS member_certifications_member_idx
    ON member_certifications (member_id, sort_order);

CREATE TABLE IF NOT EXISTS member_portfolio_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    title           TEXT NOT NULL,
    url             TEXT NOT NULL,
    kind            TEXT NOT NULL DEFAULT 'link',
    showcase_slug   TEXT NOT NULL DEFAULT '',
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS member_portfolio_links_member_idx
    ON member_portfolio_links (member_id, sort_order);

-- Skills with evidence: self_declared → endorsed → verified (completed work).
CREATE TABLE IF NOT EXISTS member_skills (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    skill_id        UUID REFERENCES showcase_skills(id) ON DELETE SET NULL,
    skill_name      TEXT NOT NULL,
    evidence_level  TEXT NOT NULL DEFAULT 'self_declared'
        CHECK (evidence_level IN ('self_declared', 'endorsed', 'verified')),
    proficiency     INT NOT NULL DEFAULT 5 CHECK (proficiency BETWEEN 1 AND 10),
    endorsement_count INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (member_id, skill_name)
);
CREATE INDEX IF NOT EXISTS member_skills_member_idx ON member_skills (member_id);
CREATE INDEX IF NOT EXISTS member_skills_name_idx ON member_skills (lower(skill_name));

CREATE TABLE IF NOT EXISTS member_skill_endorsements (
    skill_row_id    UUID NOT NULL REFERENCES member_skills(id) ON DELETE CASCADE,
    endorser_id     UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (skill_row_id, endorser_id)
);

-- Mutual connections (accepted). Pending requests live here with status.
CREATE TABLE IF NOT EXISTS member_connections (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    requester_id    UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    addressee_id    UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    status          TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'accepted', 'declined', 'blocked')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (requester_id <> addressee_id),
    UNIQUE (requester_id, addressee_id)
);
CREATE INDEX IF NOT EXISTS member_connections_addressee_idx
    ON member_connections (addressee_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS member_connections_requester_idx
    ON member_connections (requester_id, status, created_at DESC);

CREATE TABLE IF NOT EXISTS member_reviews (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    subject_id      UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    author_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    rating          INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    body            TEXT NOT NULL DEFAULT '',
    engagement_id   UUID,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (subject_id <> author_id)
);
CREATE INDEX IF NOT EXISTS member_reviews_subject_idx
    ON member_reviews (subject_id, created_at DESC);

CREATE TABLE IF NOT EXISTS member_achievements (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    title           TEXT NOT NULL,
    detail          TEXT NOT NULL DEFAULT '',
    earned_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (member_id, kind)
);
CREATE INDEX IF NOT EXISTS member_achievements_member_idx
    ON member_achievements (member_id, earned_at DESC);

CREATE TABLE IF NOT EXISTS member_privacy_settings (
    member_id           UUID PRIMARY KEY REFERENCES members(id) ON DELETE CASCADE,
    show_email_to       TEXT NOT NULL DEFAULT 'nobody'
        CHECK (show_email_to IN ('nobody', 'connections', 'hirers')),
    show_location       BOOLEAN NOT NULL DEFAULT TRUE,
    show_rate           BOOLEAN NOT NULL DEFAULT TRUE,
    show_availability   BOOLEAN NOT NULL DEFAULT TRUE,
    searchable          BOOLEAN NOT NULL DEFAULT TRUE,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dashboard_layouts (
    member_id       UUID PRIMARY KEY REFERENCES members(id) ON DELETE CASCADE,
    widgets         JSONB NOT NULL DEFAULT '[]',
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS conversations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    kind            TEXT NOT NULL DEFAULT 'direct'
        CHECK (kind IN ('direct', 'project', 'bid', 'requisition', 'group', 'team')),
    subject         TEXT NOT NULL DEFAULT '',
    created_by      UUID REFERENCES members(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS conversation_participants (
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    last_read_at    TIMESTAMPTZ,
    muted           BOOLEAN NOT NULL DEFAULT FALSE,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (conversation_id, member_id)
);
CREATE INDEX IF NOT EXISTS conversation_participants_member_idx
    ON conversation_participants (member_id, joined_at DESC);

CREATE TABLE IF NOT EXISTS messages (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    body            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS messages_conversation_idx
    ON messages (conversation_id, created_at DESC);

CREATE TABLE IF NOT EXISTS member_blocks (
    blocker_id      UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    blocked_id      UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (blocker_id, blocked_id),
    CHECK (blocker_id <> blocked_id)
);

CREATE TABLE IF NOT EXISTS member_reports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    reporter_id     UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    subject_id      UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    reason          TEXT NOT NULL,
    detail          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS gdpr_requests (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL CHECK (kind IN ('export', 'delete')),
    status          TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'processing', 'ready', 'done', 'cancelled')),
    payload         JSONB,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS gdpr_requests_member_idx
    ON gdpr_requests (member_id, created_at DESC);

-- Opportunity radar dismissals / feedback for ranking.
CREATE TABLE IF NOT EXISTS opportunity_feedback (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    opportunity_kind TEXT NOT NULL,
    opportunity_id  TEXT NOT NULL,
    action          TEXT NOT NULL CHECK (action IN ('save', 'dismiss', 'not_interested')),
    reason          TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (member_id, opportunity_kind, opportunity_id)
);
