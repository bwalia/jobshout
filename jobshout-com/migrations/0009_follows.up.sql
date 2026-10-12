-- Members and follows. A member is the public face of a signed-in person (or
-- a platform agent such as the Article Writer): a handle and a name, never the
-- email. Members follow each other, and follow showcase entries (apps, agents,
-- teams) for updates; /following reads both into one feed.

CREATE TABLE IF NOT EXISTS members (
    id              UUID PRIMARY KEY,
    handle          TEXT NOT NULL,
    -- Lower-cased; the join key to creator_email / author_email. Never returned.
    email           TEXT NOT NULL,
    display_name    TEXT NOT NULL DEFAULT '',
    headline        TEXT NOT NULL DEFAULT '',
    is_agent        BOOLEAN NOT NULL DEFAULT FALSE,
    follower_count  INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS members_handle_idx ON members (handle);
CREATE UNIQUE INDEX IF NOT EXISTS members_email_idx ON members (email);

CREATE TABLE IF NOT EXISTS member_follows (
    member_id       UUID NOT NULL REFERENCES members(id) ON DELETE CASCADE,
    follower_email  TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (member_id, follower_email)
);

CREATE INDEX IF NOT EXISTS member_follows_follower_idx
    ON member_follows (follower_email, created_at DESC);

CREATE TABLE IF NOT EXISTS showcase_follows (
    app_id      UUID NOT NULL REFERENCES showcase_apps(id) ON DELETE CASCADE,
    user_email  TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (app_id, user_email)
);

CREATE INDEX IF NOT EXISTS showcase_follows_user_idx
    ON showcase_follows (user_email, created_at DESC);

ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS follower_count INT NOT NULL DEFAULT 0;
-- When the creator last edited the entry while it stayed live. updated_at
-- also moves for moderation and classification, which followers don't need.
ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS revised_at TIMESTAMPTZ;
