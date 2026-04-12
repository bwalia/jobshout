-- Migration 005: Plugin system for user-defined LangGraph workflows.

CREATE TABLE IF NOT EXISTS plugins (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    version         VARCHAR(50) NOT NULL DEFAULT '1.0.0',
    description     TEXT,
    status          VARCHAR(50) NOT NULL DEFAULT 'active',
    plugin_type     VARCHAR(50) NOT NULL DEFAULT 'langgraph',
    -- The workflow definition: nodes, edges, entry_point (stored as JSON).
    workflow_def    JSONB NOT NULL DEFAULT '{}',
    -- Permissions declared by the plugin.
    permissions     JSONB NOT NULL DEFAULT '["llm_access"]',
    -- Additional config (requirements, env vars, etc.).
    config          JSONB NOT NULL DEFAULT '{}',
    created_by      UUID REFERENCES users(id),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plugins_org ON plugins(org_id);
CREATE INDEX IF NOT EXISTS idx_plugins_name ON plugins(org_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_plugins_org_name_version ON plugins(org_id, name, version);

-- Plugin execution history.
CREATE TABLE IF NOT EXISTS plugin_executions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    plugin_id       UUID NOT NULL REFERENCES plugins(id) ON DELETE CASCADE,
    execution_id    UUID REFERENCES agent_executions(id) ON DELETE SET NULL,
    org_id          UUID NOT NULL,
    input           JSONB NOT NULL DEFAULT '{}',
    output          TEXT,
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    error_message   TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_plugin_executions_plugin ON plugin_executions(plugin_id);
