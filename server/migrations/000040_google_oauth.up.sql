-- Platform Google sign-in / sign-up (not org-scoped SSO, not Gmail Mail Agent).
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table.

ALTER TABLE users
    ADD COLUMN IF NOT EXISTS google_sub TEXT;

-- One Google account maps to one user. NULLs (password-only users) are allowed.
CREATE UNIQUE INDEX IF NOT EXISTS users_google_sub_uidx
    ON users (google_sub)
    WHERE google_sub IS NOT NULL;

-- CSRF state for the Google redirect. Consumed (deleted) on callback.
CREATE TABLE IF NOT EXISTS auth_oauth_states (
    state      TEXT PRIMARY KEY,
    intent     TEXT NOT NULL DEFAULT 'login',
    org_name   TEXT NOT NULL DEFAULT '',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- One-time ticket: the API issues JWTs only after the frontend POSTs this.
-- Tokens never appear in the browser redirect URL.
CREATE TABLE IF NOT EXISTS auth_oauth_tickets (
    ticket     TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
