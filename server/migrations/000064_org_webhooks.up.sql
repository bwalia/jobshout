-- Migration 064: outbound webhooks (org-configured endpoints + delivery log).
--
-- An org subscribes an endpoint (URL + signing secret) to event types
-- (run.completed, run.failed, approval.requested, approval.decided; "*" = all).
-- Each event becomes one delivery row per subscribed endpoint; deliveries are
-- POSTed with an HMAC-SHA256 signature and retried with backoff until they
-- succeed or run out of attempts. secret is sealed at rest when
-- SECRETS_ENCRYPTION_KEY is set.
CREATE TABLE IF NOT EXISTS org_webhook_endpoints (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    url          TEXT NOT NULL,
    secret       TEXT NOT NULL,
    events       TEXT[] NOT NULL DEFAULT '{}',
    description  TEXT NOT NULL DEFAULT '',
    enabled      BOOLEAN NOT NULL DEFAULT true,
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_org_webhook_endpoints_org ON org_webhook_endpoints(org_id);

CREATE TABLE IF NOT EXISTS org_webhook_deliveries (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    endpoint_id       UUID NOT NULL REFERENCES org_webhook_endpoints(id) ON DELETE CASCADE,
    org_id            UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    event_id          UUID NOT NULL,
    event_type        VARCHAR(64) NOT NULL,
    payload           JSONB NOT NULL,
    status            VARCHAR(16) NOT NULL DEFAULT 'pending',  -- pending | succeeded | failed
    attempts          INT NOT NULL DEFAULT 0,
    last_status_code  INT,
    last_error        TEXT,
    next_attempt_at   TIMESTAMPTZ,
    delivered_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_org_webhook_deliveries_endpoint ON org_webhook_deliveries(endpoint_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_org_webhook_deliveries_due ON org_webhook_deliveries(next_attempt_at) WHERE status = 'pending';
