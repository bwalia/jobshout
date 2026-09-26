-- AI Showcase: applications built by people and AI agents, at /showcase.
-- Creator claims (maturity, build method, evidence) are self-declared;
-- `verification` records what JobShout itself has checked.

CREATE TABLE IF NOT EXISTS showcase_apps (
    id                    UUID PRIMARY KEY,
    slug                  TEXT NOT NULL,
    name                  TEXT NOT NULL,
    tagline               TEXT NOT NULL DEFAULT '',
    description_md        TEXT NOT NULL DEFAULT '',
    -- Sanitised server-side from description_md; the web renders this and nothing else.
    description_html      TEXT NOT NULL DEFAULT '',
    app_type              TEXT NOT NULL DEFAULT 'other',
    maturity              TEXT NOT NULL DEFAULT 'prototype',
    build_method          TEXT NOT NULL DEFAULT 'human_ai',
    pricing               TEXT NOT NULL DEFAULT 'free',
    license               TEXT NOT NULL DEFAULT '',
    version               TEXT NOT NULL DEFAULT '',
    logo_url              TEXT NOT NULL DEFAULT '',
    screenshots           TEXT[] NOT NULL DEFAULT '{}',
    repo_url              TEXT NOT NULL DEFAULT '',
    demo_url              TEXT NOT NULL DEFAULT '',
    website_url           TEXT NOT NULL DEFAULT '',
    docs_url              TEXT NOT NULL DEFAULT '',
    technologies          TEXT[] NOT NULL DEFAULT '{}',
    ai_models             TEXT[] NOT NULL DEFAULT '{}',
    agents                JSONB NOT NULL DEFAULT '[]',
    human_oversight       TEXT NOT NULL DEFAULT '',
    evidence              JSONB NOT NULL DEFAULT '{}',
    team_name             TEXT NOT NULL DEFAULT '',
    creator_email         TEXT NOT NULL,
    creator_display_name  TEXT NOT NULL DEFAULT '',
    visibility            TEXT NOT NULL DEFAULT 'public',
    status                TEXT NOT NULL DEFAULT 'draft',
    source                TEXT NOT NULL DEFAULT 'community',
    verification          TEXT NOT NULL DEFAULT 'unverified',
    featured              BOOLEAN NOT NULL DEFAULT FALSE,
    review_note           TEXT NOT NULL DEFAULT '',
    star_count            INT NOT NULL DEFAULT 0,
    -- Technologies, models and agent names flattened by the API, so search
    -- finds "rust" or "claude" (array_to_string is not immutable enough for
    -- a generated column).
    tags_text             TEXT NOT NULL DEFAULT '',
    published_at          TIMESTAMPTZ,
    created_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at            TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search                TSVECTOR GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(tagline, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(tags_text, '')), 'B') ||
        setweight(to_tsvector('english', coalesce(description_md, '')), 'C')
    ) STORED
);

CREATE UNIQUE INDEX IF NOT EXISTS showcase_apps_slug_idx ON showcase_apps (slug);
CREATE INDEX IF NOT EXISTS showcase_apps_feed_idx
    ON showcase_apps (status, visibility, published_at DESC);
CREATE INDEX IF NOT EXISTS showcase_apps_creator_idx
    ON showcase_apps (lower(creator_email), created_at DESC);
CREATE INDEX IF NOT EXISTS showcase_apps_search_idx
    ON showcase_apps USING GIN (search);
CREATE INDEX IF NOT EXISTS showcase_apps_tech_idx
    ON showcase_apps USING GIN (technologies);

CREATE TABLE IF NOT EXISTS showcase_stars (
    app_id      UUID NOT NULL REFERENCES showcase_apps(id) ON DELETE CASCADE,
    user_email  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, user_email)
);

CREATE INDEX IF NOT EXISTS showcase_stars_user_idx ON showcase_stars (user_email, created_at DESC);
