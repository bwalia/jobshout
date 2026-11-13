-- Phase 6: Teams & agencies + analytics_daily rollups. Flag TEAMS_ENABLED.

CREATE TABLE IF NOT EXISTS marketplace_teams (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug            TEXT NOT NULL UNIQUE,
    name            TEXT NOT NULL,
    summary         TEXT NOT NULL DEFAULT '',
    agency_profile  BOOLEAN NOT NULL DEFAULT FALSE,
    owner_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    pooled_credits_account_id UUID REFERENCES accounts(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS marketplace_team_members (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    team_id         UUID NOT NULL REFERENCES marketplace_teams(id) ON DELETE CASCADE,
    -- Human member OR showcase agent (one of the two set).
    member_id       UUID REFERENCES members(id) ON DELETE CASCADE,
    showcase_app_id UUID,
    role            TEXT NOT NULL DEFAULT 'member',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (member_id IS NOT NULL OR showcase_app_id IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS marketplace_team_members_team_idx
    ON marketplace_team_members (team_id);

ALTER TABLE bids
    ADD COLUMN IF NOT EXISTS team_id UUID REFERENCES marketplace_teams(id) ON DELETE SET NULL;

CREATE TABLE IF NOT EXISTS analytics_daily (
    day             DATE NOT NULL,
    subject_type    TEXT NOT NULL,
    subject_id      TEXT NOT NULL,
    metric          TEXT NOT NULL,
    value           DOUBLE PRECISION NOT NULL DEFAULT 0,
    PRIMARY KEY (day, subject_type, subject_id, metric)
);
CREATE INDEX IF NOT EXISTS analytics_daily_subject_idx
    ON analytics_daily (subject_type, subject_id, day DESC);
