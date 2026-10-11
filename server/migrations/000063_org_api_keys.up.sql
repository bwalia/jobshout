-- Migration 063: org-scoped API keys for service principals (e.g. OpsAPI).
--
-- Only a SHA-256 hash of the key is stored; the full key is shown once, at
-- creation. prefix ("jsk_" + 8 hex chars) is safe to display and identifies a
-- key in lists and logs. A key acts in its org as the user who created it, so
-- it stops working when that user is removed or leaves the org.
CREATE TABLE IF NOT EXISTS org_api_keys (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name          VARCHAR(100) NOT NULL,
    prefix        VARCHAR(32) NOT NULL,
    key_hash      TEXT NOT NULL,
    scopes        TEXT[] NOT NULL DEFAULT '{}',
    created_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    last_used_at  TIMESTAMPTZ,
    expires_at    TIMESTAMPTZ,
    revoked_at    TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_org_api_keys_hash ON org_api_keys(key_hash);
CREATE INDEX IF NOT EXISTS idx_org_api_keys_org ON org_api_keys(org_id, created_at DESC);
