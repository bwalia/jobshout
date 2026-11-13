-- Phase 4: Linked Groups. Flag GROUPS_ENABLED. Secret groups stay out of search.

CREATE TABLE IF NOT EXISTS community_groups (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    description     TEXT NOT NULL DEFAULT '',
    kind            TEXT NOT NULL DEFAULT 'skill'
        CHECK (kind IN ('skill', 'industry', 'location', 'company', 'ai_agent')),
    visibility      TEXT NOT NULL DEFAULT 'public'
        CHECK (visibility IN ('public', 'private', 'secret')),
    join_policy     TEXT NOT NULL DEFAULT 'open'
        CHECK (join_policy IN ('open', 'request', 'invite')),
    owner_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    rules           TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS community_groups_visibility_idx
    ON community_groups (visibility, kind) WHERE visibility <> 'secret';

CREATE TABLE IF NOT EXISTS community_group_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id        UUID NOT NULL REFERENCES community_groups(id) ON DELETE CASCADE,
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    role            TEXT NOT NULL DEFAULT 'member'
        CHECK (role IN ('owner', 'moderator', 'member')),
    banned          BOOLEAN NOT NULL DEFAULT FALSE,
    discoverable    BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (group_id, member_id)
);
CREATE INDEX IF NOT EXISTS community_group_members_member_idx
    ON community_group_members (member_id) WHERE NOT banned;

CREATE TABLE IF NOT EXISTS community_group_posts (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id        UUID NOT NULL REFERENCES community_groups(id) ON DELETE CASCADE,
    author_member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL DEFAULT 'discussion'
        CHECK (kind IN ('discussion', 'question', 'opportunity', 'event', 'announcement')),
    title           TEXT NOT NULL DEFAULT '',
    body            TEXT NOT NULL,
    pinned          BOOLEAN NOT NULL DEFAULT FALSE,
    locked          BOOLEAN NOT NULL DEFAULT FALSE,
    reference_type  TEXT NOT NULL DEFAULT '',
    reference_id    TEXT NOT NULL DEFAULT '',
    event_starts_at TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS community_group_posts_group_idx
    ON community_group_posts (group_id, pinned DESC, created_at DESC);

CREATE TABLE IF NOT EXISTS community_group_comments (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    post_id         UUID NOT NULL REFERENCES community_group_posts(id) ON DELETE CASCADE,
    author_member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    body            TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS community_group_reactions (
    post_id         UUID NOT NULL REFERENCES community_group_posts(id) ON DELETE CASCADE,
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    reaction        TEXT NOT NULL DEFAULT 'like',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (post_id, member_id, reaction)
);

CREATE TABLE IF NOT EXISTS community_group_reports (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id        UUID NOT NULL REFERENCES community_groups(id) ON DELETE CASCADE,
    reporter_member_id UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    target_type     TEXT NOT NULL,
    target_id       UUID NOT NULL,
    reason          TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'open'
        CHECK (status IN ('open', 'resolved', 'dismissed')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Official seed groups (idempotent).
INSERT INTO community_groups (slug, name, description, kind, visibility, join_policy, rules)
SELECT * FROM (VALUES
    ('rust-builders', 'Rust Builders', 'People shipping Rust in production.', 'skill', 'public', 'open', 'Be kind. Share code, not credentials.'),
    ('uk-tech', 'UK Tech', 'UK industry discussions and opportunities.', 'industry', 'public', 'open', 'UK-focused; no spam.'),
    ('ai-agents', 'AI Agents', 'Builders of agents and agent teams.', 'ai_agent', 'public', 'open', 'Evidence over hype.')
) AS v(slug, name, description, kind, visibility, join_policy, rules)
WHERE NOT EXISTS (SELECT 1 FROM community_groups g WHERE g.slug = v.slug);
