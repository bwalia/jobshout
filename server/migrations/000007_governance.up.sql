-- 000007_governance.up.sql
-- Enterprise agent governance: cost tracking, budgets, policies, usage analytics.

-- ─── Extend agent_executions with cost/token breakdown ──────────────────────
ALTER TABLE agent_executions
    ADD COLUMN IF NOT EXISTS input_tokens    INTEGER       NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS output_tokens   INTEGER       NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS latency_ms      INTEGER       NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS cost_usd        NUMERIC(12,8) NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS model_name      VARCHAR(100),
    ADD COLUMN IF NOT EXISTS model_provider  VARCHAR(100);

CREATE INDEX IF NOT EXISTS idx_agent_executions_org_created
    ON agent_executions(org_id, created_at);
CREATE INDEX IF NOT EXISTS idx_agent_executions_agent_created
    ON agent_executions(agent_id, created_at);

-- ─── Org-level budgets (hard/soft limits per billing period) ────────────────
CREATE TABLE IF NOT EXISTS org_budgets (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    period          VARCHAR(20)  NOT NULL DEFAULT 'monthly',
    soft_limit_usd  NUMERIC(12,2),
    hard_limit_usd  NUMERIC(12,2),
    alert_threshold NUMERIC(5,2) NOT NULL DEFAULT 0.80,
    enabled         BOOLEAN      NOT NULL DEFAULT true,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, period)
);

-- ─── Agent policies (per-agent or org-wide governance rules) ────────────────
CREATE TABLE IF NOT EXISTS agent_policies (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id              UUID        NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id            UUID        REFERENCES agents(id) ON DELETE CASCADE,
    max_tokens_per_exec INTEGER,
    allowed_models      TEXT[],
    allowed_providers   TEXT[],
    max_cost_per_exec   NUMERIC(12,6),
    max_execs_per_day   INTEGER,
    max_execs_per_hour  INTEGER,
    enabled             BOOLEAN     NOT NULL DEFAULT true,
    created_at          TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Unique index: one policy per (org, agent) pair; NULL agent_id = org-wide default.
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_policies_org_agent
    ON agent_policies(org_id, COALESCE(agent_id, '00000000-0000-0000-0000-000000000000'::uuid));

-- ─── Pre-aggregated usage rollups for analytics queries ─────────────────────
CREATE TABLE IF NOT EXISTS usage_rollups (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID          NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        UUID          REFERENCES agents(id) ON DELETE SET NULL,
    model_provider  VARCHAR(100)  NOT NULL DEFAULT '',
    model_name      VARCHAR(100)  NOT NULL DEFAULT '',
    period_type     VARCHAR(20)   NOT NULL DEFAULT 'daily',
    period_start    TIMESTAMPTZ   NOT NULL,
    exec_count      INTEGER       NOT NULL DEFAULT 0,
    input_tokens    BIGINT        NOT NULL DEFAULT 0,
    output_tokens   BIGINT        NOT NULL DEFAULT 0,
    total_tokens    BIGINT        NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(14,8) NOT NULL DEFAULT 0,
    avg_latency_ms  INTEGER       NOT NULL DEFAULT 0,
    error_count     INTEGER       NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, agent_id, model_provider, model_name, period_type, period_start)
);

CREATE INDEX IF NOT EXISTS idx_usage_rollups_org_period
    ON usage_rollups(org_id, period_start);

-- ─── Budget alert audit log ─────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS budget_alerts (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    budget_id    UUID         NOT NULL REFERENCES org_budgets(id) ON DELETE CASCADE,
    alert_type   VARCHAR(50)  NOT NULL,
    spend_usd    NUMERIC(12,4) NOT NULL,
    limit_usd    NUMERIC(12,4) NOT NULL,
    triggered_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_budget_alerts_org_triggered
    ON budget_alerts(org_id, triggered_at);
