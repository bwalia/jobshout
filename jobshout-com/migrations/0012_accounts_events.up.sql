-- Phase 0 foundation: accounts/roles, event spine, follow dual-key by member id.
-- Additive only; email columns stay for dual-read during transition.

CREATE TABLE IF NOT EXISTS accounts (
    id              UUID PRIMARY KEY,
    -- Always stored lower-cased by the API.
    primary_email   TEXT NOT NULL,
    display_name    TEXT NOT NULL DEFAULT '',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS accounts_primary_email_idx
    ON accounts (lower(primary_email));

CREATE TABLE IF NOT EXISTS account_identities (
    id              UUID PRIMARY KEY,
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    provider        TEXT NOT NULL,
    provider_subject TEXT NOT NULL,
    email           TEXT NOT NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (provider, provider_subject)
);

CREATE INDEX IF NOT EXISTS account_identities_account_idx
    ON account_identities (account_id);

CREATE INDEX IF NOT EXISTS account_identities_email_idx
    ON account_identities (lower(email));

-- Roles: member, freelancer, job_seeker, recruiter, employer, agency, agent_owner, staff
CREATE TABLE IF NOT EXISTS account_roles (
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    role            TEXT NOT NULL,
    granted_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (account_id, role)
);

ALTER TABLE members
    ADD COLUMN IF NOT EXISTS account_id UUID REFERENCES accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS members_account_idx ON members (account_id);

ALTER TABLE candidate_profiles
    ADD COLUMN IF NOT EXISTS account_id UUID REFERENCES accounts(id) ON DELETE SET NULL;

CREATE INDEX IF NOT EXISTS candidate_profiles_account_idx
    ON candidate_profiles (account_id);

-- Follows re-keyed by member id (dual-read with email columns).
ALTER TABLE member_follows
    ADD COLUMN IF NOT EXISTS follower_member_id UUID REFERENCES members(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS member_follows_follower_member_idx
    ON member_follows (follower_member_id, created_at DESC);

ALTER TABLE showcase_follows
    ADD COLUMN IF NOT EXISTS follower_member_id UUID REFERENCES members(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS showcase_follows_follower_member_idx
    ON showcase_follows (follower_member_id, created_at DESC);

-- Event spine
CREATE TABLE IF NOT EXISTS domain_events (
    id              UUID PRIMARY KEY,
    event_type      TEXT NOT NULL,
    aggregate_type  TEXT NOT NULL DEFAULT '',
    aggregate_id    TEXT NOT NULL DEFAULT '',
    actor_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS domain_events_type_created_idx
    ON domain_events (event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS domain_events_aggregate_idx
    ON domain_events (aggregate_type, aggregate_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notifications (
    id              UUID PRIMARY KEY,
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL,
    title           TEXT NOT NULL,
    body            TEXT NOT NULL DEFAULT '',
    href            TEXT NOT NULL DEFAULT '',
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
    read_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS notifications_account_created_idx
    ON notifications (account_id, created_at DESC);

CREATE TABLE IF NOT EXISTS notification_preferences (
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    channel         TEXT NOT NULL,
    kind            TEXT NOT NULL,
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    PRIMARY KEY (account_id, channel, kind)
);

CREATE TABLE IF NOT EXISTS email_outbox (
    id              UUID PRIMARY KEY,
    to_email        TEXT NOT NULL,
    subject         TEXT NOT NULL,
    body_text       TEXT NOT NULL DEFAULT '',
    body_html       TEXT NOT NULL DEFAULT '',
    status          TEXT NOT NULL DEFAULT 'pending',
    attempts        INT NOT NULL DEFAULT 0,
    max_attempts    INT NOT NULL DEFAULT 8,
    last_error      TEXT NOT NULL DEFAULT '',
    scheduled_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at         TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS email_outbox_pending_idx
    ON email_outbox (status, scheduled_at)
    WHERE status IN ('pending', 'retry');

-- Backfill accounts from every known email column (lower-cased).
INSERT INTO accounts (id, primary_email, display_name)
SELECT gen_random_uuid(), e.email, COALESCE(NULLIF(e.display_name, ''), split_part(e.email, '@', 1))
FROM (
    SELECT lower(email) AS email, MAX(display_name) AS display_name
    FROM (
        SELECT email, display_name FROM members WHERE email <> ''
        UNION ALL
        SELECT email, display_name FROM candidate_profiles WHERE email <> ''
        UNION ALL
        SELECT email, full_name FROM job_applications WHERE email <> ''
        UNION ALL
        SELECT poster_email, '' FROM jobs WHERE poster_email <> ''
        UNION ALL
        SELECT creator_email, creator_display_name FROM showcase_apps WHERE creator_email <> ''
        UNION ALL
        SELECT author_email, author_display_name FROM insights_items WHERE author_email <> ''
        UNION ALL
        SELECT email, '' FROM insights_newsletter_subscribers WHERE email <> ''
        UNION ALL
        SELECT follower_email, '' FROM member_follows WHERE follower_email <> ''
        UNION ALL
        SELECT user_email, '' FROM showcase_follows WHERE user_email <> ''
    ) raw
    GROUP BY lower(email)
) e
WHERE NOT EXISTS (
    SELECT 1 FROM accounts a WHERE lower(a.primary_email) = e.email
);

INSERT INTO account_identities (id, account_id, provider, provider_subject, email)
SELECT gen_random_uuid(), a.id, 'email', lower(a.primary_email), lower(a.primary_email)
FROM accounts a
WHERE NOT EXISTS (
    SELECT 1 FROM account_identities i
    WHERE i.provider = 'email' AND i.provider_subject = lower(a.primary_email)
);

INSERT INTO account_roles (account_id, role)
SELECT a.id, 'member'
FROM accounts a
WHERE NOT EXISTS (
    SELECT 1 FROM account_roles r WHERE r.account_id = a.id AND r.role = 'member'
);

UPDATE members m
SET account_id = a.id
FROM accounts a
WHERE m.account_id IS NULL AND lower(m.email) = lower(a.primary_email);

UPDATE candidate_profiles p
SET account_id = a.id
FROM accounts a
WHERE p.account_id IS NULL AND lower(p.email) = lower(a.primary_email);

-- Backfill follower_member_id where the follower already has a member row.
UPDATE member_follows f
SET follower_member_id = m.id
FROM members m
WHERE f.follower_member_id IS NULL
  AND lower(f.follower_email) = m.email;

UPDATE showcase_follows f
SET follower_member_id = m.id
FROM members m
WHERE f.follower_member_id IS NULL
  AND lower(f.user_email) = m.email;
