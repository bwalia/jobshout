-- Phase 2b: bid credits + subscriptions (ledger is source of truth).

CREATE TABLE IF NOT EXISTS credit_ledger (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    kind            TEXT NOT NULL
        CHECK (kind IN (
            'grant', 'purchase', 'subscription', 'bid', 'refund', 'expiry', 'ai', 'adjust'
        )),
    amount          INT NOT NULL,
    balance_after   INT NOT NULL,
    -- Free grants expire monthly; purchased last 12 months; NULL = no expiry.
    expires_at      TIMESTAMPTZ,
    -- Spend priority bucket: free < subscription < purchased.
    bucket          TEXT NOT NULL DEFAULT 'free'
        CHECK (bucket IN ('free', 'subscription', 'purchased')),
    idempotency_key TEXT NOT NULL,
    reference_type  TEXT NOT NULL DEFAULT '',
    reference_id    TEXT NOT NULL DEFAULT '',
    meta            JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (account_id, idempotency_key)
);
CREATE INDEX IF NOT EXISTS credit_ledger_account_idx
    ON credit_ledger (account_id, created_at DESC);
CREATE INDEX IF NOT EXISTS credit_ledger_expires_idx
    ON credit_ledger (expires_at)
    WHERE expires_at IS NOT NULL AND amount > 0;

CREATE TABLE IF NOT EXISTS credit_packs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    credits         INT NOT NULL CHECK (credits > 0),
    price_cents     INT NOT NULL CHECK (price_cents >= 0),
    currency        TEXT NOT NULL DEFAULT 'gbp',
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO credit_packs (slug, name, credits, price_cents, currency, sort_order)
SELECT * FROM (VALUES
    ('starter', 'Starter', 50, 999, 'gbp', 10),
    ('professional', 'Professional', 150, 2499, 'gbp', 20),
    ('business', 'Business', 500, 6999, 'gbp', 30)
) AS v(slug, name, credits, price_cents, currency, sort_order)
WHERE NOT EXISTS (SELECT 1 FROM credit_packs c WHERE c.slug = v.slug);

CREATE TABLE IF NOT EXISTS subscription_plans (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    monthly_credits INT NOT NULL DEFAULT 0,
    seats           INT NOT NULL DEFAULT 1,
    price_cents     INT NOT NULL CHECK (price_cents >= 0),
    currency        TEXT NOT NULL DEFAULT 'gbp',
    features        JSONB NOT NULL DEFAULT '[]'::jsonb,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    sort_order      INT NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO subscription_plans (slug, name, monthly_credits, seats, price_cents, currency, features, sort_order)
SELECT * FROM (VALUES
    (
        'freelancer_pro',
        'Freelancer Pro',
        40,
        1,
        1999,
        'gbp',
        '["monthly_credits","analytics","unlimited_ai_proposals"]'::jsonb,
        10
    ),
    (
        'agency',
        'Agency',
        200,
        5,
        9999,
        'gbp',
        '["seats","pooled_credits","analytics"]'::jsonb,
        20
    ),
    (
        'recruiter',
        'Recruiter',
        0,
        3,
        7999,
        'gbp',
        '["hire_seats","saved_searches","analytics"]'::jsonb,
        30
    )
) AS v(slug, name, monthly_credits, seats, price_cents, currency, features, sort_order)
WHERE NOT EXISTS (SELECT 1 FROM subscription_plans s WHERE s.slug = v.slug);

CREATE TABLE IF NOT EXISTS account_subscriptions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    plan_id         UUID NOT NULL REFERENCES subscription_plans(id),
    status          TEXT NOT NULL DEFAULT 'active'
        CHECK (status IN ('active', 'past_due', 'cancelled', 'trialing')),
    stripe_subscription_id TEXT NOT NULL DEFAULT '',
    current_period_end TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS account_subscriptions_one_active_idx
    ON account_subscriptions (account_id)
    WHERE status IN ('active', 'trialing', 'past_due');

CREATE TABLE IF NOT EXISTS stripe_webhook_events (
    id              TEXT PRIMARY KEY,
    type            TEXT NOT NULL,
    processed_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    payload         JSONB NOT NULL DEFAULT '{}'::jsonb
);

CREATE TABLE IF NOT EXISTS bid_daily_caps (
    account_id      UUID NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    day             DATE NOT NULL,
    bids_count      INT NOT NULL DEFAULT 0,
    PRIMARY KEY (account_id, day)
);

-- Featured project listing (paid) — flag-gated purchases later.
ALTER TABLE projects
    ADD COLUMN IF NOT EXISTS featured_listing_until TIMESTAMPTZ;
