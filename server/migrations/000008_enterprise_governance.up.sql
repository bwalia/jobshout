-- 000008_enterprise_governance.up.sql
-- Enhanced enterprise governance: RBAC, SSO, pricing configs, audit logs,
-- usage records (partitioned), cost records, agent scoring.

-- ─── Roles & Permissions (RBAC) ─────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(50)  NOT NULL,
    description TEXT,
    permissions TEXT[]       NOT NULL DEFAULT '{}',
    is_system   BOOLEAN      NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, name)
);

CREATE TABLE IF NOT EXISTS user_roles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role_id     UUID         NOT NULL REFERENCES roles(id) ON DELETE CASCADE,
    org_id      UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    granted_by  UUID         REFERENCES users(id) ON DELETE SET NULL,
    granted_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE(user_id, role_id, org_id)
);

CREATE INDEX IF NOT EXISTS idx_user_roles_user ON user_roles(user_id);
CREATE INDEX IF NOT EXISTS idx_user_roles_org ON user_roles(org_id);

-- ─── SSO / OIDC Configurations ──────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS sso_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID          NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    provider        VARCHAR(50)   NOT NULL,
    client_id       VARCHAR(255)  NOT NULL,
    client_secret   VARCHAR(500)  NOT NULL,
    issuer_url      VARCHAR(500)  NOT NULL,
    redirect_url    VARCHAR(500)  NOT NULL,
    scopes          TEXT[]        NOT NULL DEFAULT '{openid,profile,email}',
    auto_provision  BOOLEAN       NOT NULL DEFAULT true,
    default_role    VARCHAR(50)   NOT NULL DEFAULT 'viewer',
    domain_filter   VARCHAR(255),
    enabled         BOOLEAN       NOT NULL DEFAULT true,
    metadata        JSONB         NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, provider)
);

-- ─── SSO Login Audit ────────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS login_audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID         REFERENCES users(id) ON DELETE SET NULL,
    org_id      UUID         REFERENCES organizations(id) ON DELETE SET NULL,
    email       VARCHAR(255),
    provider    VARCHAR(50)  NOT NULL DEFAULT 'local',
    ip_address  VARCHAR(45),
    user_agent  TEXT,
    status      VARCHAR(20)  NOT NULL,
    error_msg   TEXT,
    metadata    JSONB        NOT NULL DEFAULT '{}',
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_login_audit_user ON login_audit_logs(user_id, created_at);
CREATE INDEX IF NOT EXISTS idx_login_audit_org ON login_audit_logs(org_id, created_at);

-- ─── Pricing Configurations (versioned, tenant-overridable) ─────────────────

CREATE TABLE IF NOT EXISTS pricing_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID         REFERENCES organizations(id) ON DELETE CASCADE,
    provider        VARCHAR(100) NOT NULL,
    model           VARCHAR(100) NOT NULL,
    input_price_per_m_token  NUMERIC(12,6) NOT NULL DEFAULT 0,
    output_price_per_m_token NUMERIC(12,6) NOT NULL DEFAULT 0,
    compute_price_per_sec    NUMERIC(12,6) NOT NULL DEFAULT 0,
    version         INTEGER      NOT NULL DEFAULT 1,
    effective_from  TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    effective_until TIMESTAMPTZ,
    is_active       BOOLEAN      NOT NULL DEFAULT true,
    metadata        JSONB        NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- org_id NULL = system-wide default; non-NULL = tenant override
CREATE INDEX IF NOT EXISTS idx_pricing_active
    ON pricing_configs(provider, model, is_active)
    WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_pricing_org
    ON pricing_configs(org_id, provider, model)
    WHERE org_id IS NOT NULL;

-- ─── Usage Records (partitioned by month for scale) ─────────────────────────

CREATE TABLE IF NOT EXISTS usage_records (
    id              UUID         NOT NULL DEFAULT gen_random_uuid(),
    org_id          UUID         NOT NULL,
    agent_id        UUID,
    execution_id    UUID,
    task_id         UUID,
    user_id         UUID,
    provider        VARCHAR(100) NOT NULL DEFAULT '',
    model           VARCHAR(100) NOT NULL DEFAULT '',
    tokens_in       INTEGER      NOT NULL DEFAULT 0,
    tokens_out      INTEGER      NOT NULL DEFAULT 0,
    latency_ms      INTEGER      NOT NULL DEFAULT 0,
    cost_usd        NUMERIC(12,8) NOT NULL DEFAULT 0,
    tool_calls      INTEGER      NOT NULL DEFAULT 0,
    retries         INTEGER      NOT NULL DEFAULT 0,
    is_error        BOOLEAN      NOT NULL DEFAULT false,
    metadata        JSONB        NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    PRIMARY KEY (id, created_at)
) PARTITION BY RANGE (created_at);

-- Create partitions for current and next 3 months (auto-extend in production)
DO $$
DECLARE
    start_date DATE := DATE_TRUNC('month', CURRENT_DATE);
    end_date   DATE;
    part_name  TEXT;
BEGIN
    FOR i IN 0..3 LOOP
        end_date   := start_date + INTERVAL '1 month';
        part_name  := 'usage_records_' || TO_CHAR(start_date, 'YYYY_MM');
        EXECUTE format(
            'CREATE TABLE IF NOT EXISTS %I PARTITION OF usage_records
             FOR VALUES FROM (%L) TO (%L)',
            part_name, start_date, end_date
        );
        start_date := end_date;
    END LOOP;
END $$;

CREATE INDEX IF NOT EXISTS idx_usage_records_org_created
    ON usage_records(org_id, created_at);
CREATE INDEX IF NOT EXISTS idx_usage_records_agent
    ON usage_records(agent_id, created_at)
    WHERE agent_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_usage_records_exec
    ON usage_records(execution_id)
    WHERE execution_id IS NOT NULL;

-- ─── Cost Records (task-level attribution) ──────────────────────────────────

CREATE TABLE IF NOT EXISTS cost_records (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID          NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    execution_id    UUID,
    task_id         UUID,
    agent_id        UUID,
    cost_type       VARCHAR(50)   NOT NULL DEFAULT 'llm',
    llm_cost_usd    NUMERIC(12,8) NOT NULL DEFAULT 0,
    tool_cost_usd   NUMERIC(12,8) NOT NULL DEFAULT 0,
    compute_cost_usd NUMERIC(12,8) NOT NULL DEFAULT 0,
    total_cost_usd  NUMERIC(12,8) NOT NULL DEFAULT 0,
    provider        VARCHAR(100)  NOT NULL DEFAULT '',
    model           VARCHAR(100)  NOT NULL DEFAULT '',
    breakdown       JSONB         NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_cost_records_org_created
    ON cost_records(org_id, created_at);
CREATE INDEX IF NOT EXISTS idx_cost_records_task
    ON cost_records(task_id) WHERE task_id IS NOT NULL;

-- ─── General Audit Log ──────────────────────────────────────────────────────

CREATE TABLE IF NOT EXISTS audit_logs (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID         NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id     UUID         REFERENCES users(id) ON DELETE SET NULL,
    action      VARCHAR(100) NOT NULL,
    resource    VARCHAR(100) NOT NULL,
    resource_id UUID,
    cost_usd    NUMERIC(12,8),
    old_value   JSONB,
    new_value   JSONB,
    metadata    JSONB        NOT NULL DEFAULT '{}',
    ip_address  VARCHAR(45),
    created_at  TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_logs_org_created
    ON audit_logs(org_id, created_at);
CREATE INDEX IF NOT EXISTS idx_audit_logs_resource
    ON audit_logs(org_id, resource, resource_id);

-- ─── Agent Scoring (for intelligent selection) ──────────────────────────────

CREATE TABLE IF NOT EXISTS agent_scores (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID          NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        UUID          NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_type       VARCHAR(100)  NOT NULL DEFAULT 'general',
    success_rate    NUMERIC(5,4)  NOT NULL DEFAULT 0,
    avg_latency_ms  INTEGER       NOT NULL DEFAULT 0,
    avg_cost_usd    NUMERIC(12,8) NOT NULL DEFAULT 0,
    total_runs      INTEGER       NOT NULL DEFAULT 0,
    score           NUMERIC(8,4)  NOT NULL DEFAULT 0,
    last_updated    TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    UNIQUE(org_id, agent_id, task_type)
);

CREATE INDEX IF NOT EXISTS idx_agent_scores_org_score
    ON agent_scores(org_id, task_type, score DESC);

-- ─── Add metadata columns to existing tables ────────────────────────────────

ALTER TABLE organizations ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';
ALTER TABLE agents        ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';
ALTER TABLE tasks         ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}';
