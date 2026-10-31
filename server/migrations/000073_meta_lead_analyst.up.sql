-- Migration 073: Meta Lead Analyst.
--
-- Named Page connections (token AES-GCM sealed with META_TOKEN_KEY), analysis
-- runs (launch or webhook), and per-lead field data + LLM briefs.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

CREATE TABLE IF NOT EXISTS meta_lead_connections (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id           UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name             VARCHAR(128) NOT NULL,
    page_id          VARCHAR(64)  NOT NULL,
    page_name        VARCHAR(255) NOT NULL DEFAULT '',
    access_token_enc BYTEA        NOT NULL,
    connected_by     UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at       TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_meta_lead_connections_page
    ON meta_lead_connections (page_id);

CREATE TABLE IF NOT EXISTS meta_lead_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    task_id       UUID,
    connection_id UUID NOT NULL REFERENCES meta_lead_connections(id) ON DELETE CASCADE,
    form_id       VARCHAR(64)  NOT NULL DEFAULT '',
    form_name     VARCHAR(255) NOT NULL DEFAULT '',
    focus         TEXT         NOT NULL DEFAULT '',
    status        VARCHAR(32)  NOT NULL DEFAULT 'running',
    lead_count    INT          NOT NULL DEFAULT 0,
    rollup        JSONB,
    error_message TEXT         NOT NULL DEFAULT '',
    source        VARCHAR(32)  NOT NULL DEFAULT 'launch',
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_meta_lead_runs_org_created
    ON meta_lead_runs (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS meta_leads (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    connection_id UUID NOT NULL REFERENCES meta_lead_connections(id) ON DELETE CASCADE,
    run_id        UUID REFERENCES meta_lead_runs(id) ON DELETE SET NULL,
    leadgen_id    VARCHAR(64) NOT NULL,
    form_id       VARCHAR(64) NOT NULL DEFAULT '',
    field_data    JSONB,
    created_time  TIMESTAMPTZ,
    analysis      JSONB,
    status        VARCHAR(32) NOT NULL DEFAULT 'pending',
    error_message TEXT        NOT NULL DEFAULT '',
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, leadgen_id)
);

CREATE INDEX IF NOT EXISTS idx_meta_leads_org_created
    ON meta_leads (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_meta_leads_run
    ON meta_leads (run_id);

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Meta Lead Analyst',
    'Lead Analyst',
    'Pulls Instagram and Facebook Lead Ads Instant Form submissions via the Meta Graph API and runs LLM analysis: a brief per lead (intent, fit, next action, spam risk) plus a rollup for the run. New leads can also arrive through the leadgen webhook.',
    'active',
    'go_native',
    'You are the Meta Lead Analyst. You read Instant Form field data only — never invent contact details — and write clear, actionable qualification notes for sales and ops.',
    '{"builtin":"meta_lead_analyst"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'meta_lead_analyst'
);
