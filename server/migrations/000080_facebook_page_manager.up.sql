-- Migration 080: the Facebook Page Manager.
--
-- facebook_connections: the Facebook user an org connected. Only identity and
-- expiry are kept; the user token is used once to list Pages and is not stored.
--
-- facebook_pages: each Page the user administers, with its own sealed Page
-- access token (AES-256-GCM with FACEBOOK_TOKEN_KEY) and its own brand profile,
-- so several Pages are managed independently. needs_reauth is set when Meta
-- rejects the token (revoked or expired).
--
-- facebook_oauth_states: one-time OAuth state for the connect flow.
--
-- facebook_posts: one row per calendar post. Lifecycle:
--   drafting -> draft -> scheduled -> publishing -> published   (failed anywhere)
-- scheduled posts are published by the server when scheduled_for is due.
--
-- Also seeds the agent for existing orgs; new orgs get it from the module
-- registry at sign-up.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

CREATE TABLE IF NOT EXISTS facebook_connections (
    org_id       UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    fb_user_id   VARCHAR(64)  NOT NULL DEFAULT '',
    name         VARCHAR(255) NOT NULL DEFAULT '',
    connected_by UUID REFERENCES users(id) ON DELETE SET NULL,
    expires_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS facebook_pages (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    page_id      VARCHAR(64)  NOT NULL,
    name         VARCHAR(255) NOT NULL DEFAULT '',
    category     VARCHAR(255) NOT NULL DEFAULT '',
    about        TEXT NOT NULL DEFAULT '',
    description  TEXT NOT NULL DEFAULT '',
    website      TEXT NOT NULL DEFAULT '',
    followers    INTEGER NOT NULL DEFAULT 0,
    token_enc    BYTEA NOT NULL,
    needs_reauth BOOLEAN NOT NULL DEFAULT FALSE,
    brief        TEXT NOT NULL DEFAULT '',
    voice        TEXT NOT NULL DEFAULT '',
    timezone     VARCHAR(64) NOT NULL DEFAULT 'UTC',
    paused       BOOLEAN NOT NULL DEFAULT FALSE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE facebook_pages ADD COLUMN IF NOT EXISTS paused BOOLEAN NOT NULL DEFAULT FALSE;

CREATE UNIQUE INDEX IF NOT EXISTS idx_facebook_pages_org_page
    ON facebook_pages (org_id, page_id);

CREATE TABLE IF NOT EXISTS facebook_oauth_states (
    state      VARCHAR(128) PRIMARY KEY,
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS facebook_posts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    page_id       VARCHAR(64) NOT NULL,
    task_id       UUID,
    status        VARCHAR(32) NOT NULL DEFAULT 'drafting',
    message       TEXT NOT NULL DEFAULT '',
    image_url     TEXT NOT NULL DEFAULT '',
    image_prompt  TEXT NOT NULL DEFAULT '',
    category      VARCHAR(64)  NOT NULL DEFAULT '',
    audience      VARCHAR(255) NOT NULL DEFAULT '',
    objective     VARCHAR(255) NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    scheduled_for TIMESTAMPTZ,
    error_message TEXT NOT NULL DEFAULT '',
    fb_post_id    VARCHAR(128) NOT NULL DEFAULT '',
    post_url      TEXT NOT NULL DEFAULT '',
    published_at  TIMESTAMPTZ,
    approved_by   UUID,
    reactions     INTEGER NOT NULL DEFAULT 0,
    comments      INTEGER NOT NULL DEFAULT 0,
    shares        INTEGER NOT NULL DEFAULT 0,
    stats_at      TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_facebook_posts_org_created
    ON facebook_posts (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_facebook_posts_due
    ON facebook_posts (scheduled_for) WHERE status = 'scheduled';

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Facebook Page Manager',
    'Social Media Manager',
    'Plans a Facebook content calendar for a connected Page, writes the posts and their creatives, schedules and publishes them once a person approves, and reports what performed.',
    'active',
    'go_native',
    'You are the Facebook Page Manager, an experienced social media marketing manager. You write original, human-sounding Facebook posts in the Page''s brand voice and never invent testimonials, statistics, certifications, discounts or business claims.',
    '{"builtin":"facebook_page_manager"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'facebook_page_manager'
);
