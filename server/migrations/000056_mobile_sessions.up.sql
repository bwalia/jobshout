-- Migration 056: device-bound sessions and external sign-in identities.
--
-- devices: one row per signed-in app install. Refresh tokens hang off it, so
-- signing a device out (or deleting it from "Your devices") revokes exactly
-- that install's session and drops its push token with it.
--
-- user_identities: provider subjects (Sign in with Apple today) linked to a
-- user. Google keeps users.google_sub for now; new providers go here.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

CREATE TABLE IF NOT EXISTS devices (
    id               UUID PRIMARY KEY,
    user_id          UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    platform         VARCHAR(16)  NOT NULL,
    name             VARCHAR(255) NOT NULL DEFAULT '',
    app_version      VARCHAR(64)  NOT NULL DEFAULT '',
    push_token       TEXT,
    push_environment VARCHAR(16),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_devices_user_id ON devices(user_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_devices_push_token
    ON devices(push_token) WHERE push_token IS NOT NULL;

ALTER TABLE refresh_tokens
    ADD COLUMN IF NOT EXISTS device_id UUID REFERENCES devices(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_device_id ON refresh_tokens(device_id);

CREATE TABLE IF NOT EXISTS user_identities (
    provider   VARCHAR(32)  NOT NULL,
    subject    VARCHAR(255) NOT NULL,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    email      VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (provider, subject)
);

CREATE INDEX IF NOT EXISTS idx_user_identities_user_id ON user_identities(user_id);
