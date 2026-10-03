-- Migration 060: the LinkedIn Poster.
--
-- linkedin_connections: the LinkedIn member an org posts as. The access token
-- is AES-256-GCM sealed with LINKEDIN_TOKEN_KEY; LinkedIn gives members a
-- 60-day token and no refresh token, so expires_at is when it must be renewed.
--
-- linkedin_oauth_states: one-time OAuth state for the connect flow.
--
-- linkedin_posts: one row per article and variant. The unique key is what
-- stops two replicas, or the poller and a person, drafting the same article
-- twice: whoever inserts the row drafts it.
--
-- Also seeds the agent for existing orgs; new orgs get it from the module
-- registry at sign-up.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

CREATE TABLE IF NOT EXISTS linkedin_connections (
    org_id           UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    member_urn       VARCHAR(255) NOT NULL,
    name             VARCHAR(255) NOT NULL DEFAULT '',
    access_token_enc BYTEA        NOT NULL,
    expires_at       TIMESTAMPTZ  NOT NULL,
    connected_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS linkedin_oauth_states (
    state      VARCHAR(128) PRIMARY KEY,
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE TABLE IF NOT EXISTS linkedin_posts (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    article_id    UUID NOT NULL REFERENCES blog_articles(id) ON DELETE CASCADE,
    article_title TEXT NOT NULL DEFAULT '',
    task_id       UUID,
    variant       VARCHAR(32) NOT NULL,
    status        VARCHAR(32) NOT NULL DEFAULT 'drafting',
    commentary    TEXT NOT NULL DEFAULT '',
    link_url      TEXT NOT NULL DEFAULT '',
    notes         TEXT NOT NULL DEFAULT '',
    error_message TEXT NOT NULL DEFAULT '',
    post_urn      VARCHAR(255) NOT NULL DEFAULT '',
    post_url      TEXT NOT NULL DEFAULT '',
    posted_at     TIMESTAMPTZ,
    posted_by     UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_linkedin_posts_article_variant
    ON linkedin_posts (article_id, variant);
CREATE INDEX IF NOT EXISTS idx_linkedin_posts_org_created
    ON linkedin_posts (org_id, created_at DESC);

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'LinkedIn Poster',
    'Social Writer',
    'Turns published articles into LinkedIn posts: a technical one for engineers and a plain-language one for business readers, each about the problem the article solves. Posts go out only when a person approves them.',
    'active',
    'go_native',
    'You are the LinkedIn Poster. You turn published articles into LinkedIn posts that lead with a real problem the reader has, say what the article found, and invite discussion. You write plainly, never invent facts or numbers the article does not contain, and never use hype.',
    '{"builtin":"linkedin_poster"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'linkedin_poster'
);
