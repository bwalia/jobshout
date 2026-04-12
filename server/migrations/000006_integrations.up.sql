-- Migration 006: Third-party integration framework

-- Integration configurations (one row per connected external system)
CREATE TABLE IF NOT EXISTS integrations (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    provider        VARCHAR(50)  NOT NULL,
    base_url        TEXT NOT NULL DEFAULT '',
    credentials     JSONB NOT NULL DEFAULT '{}',
    config          JSONB NOT NULL DEFAULT '{}',
    status          VARCHAR(50)  NOT NULL DEFAULT 'active',
    last_synced_at  TIMESTAMPTZ,
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_integrations_org      ON integrations(org_id);
CREATE INDEX IF NOT EXISTS idx_integrations_provider ON integrations(org_id, provider);
CREATE UNIQUE INDEX IF NOT EXISTS idx_integrations_org_name ON integrations(org_id, name);

-- Task links (local task <-> external issue)
CREATE TABLE IF NOT EXISTS integration_task_links (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id  UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    task_id         UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    external_id     VARCHAR(255) NOT NULL,
    external_url    TEXT,
    sync_direction  VARCHAR(20)  NOT NULL DEFAULT 'bidirectional',
    last_synced_at  TIMESTAMPTZ,
    sync_status     VARCHAR(50)  NOT NULL DEFAULT 'pending',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(integration_id, task_id),
    UNIQUE(integration_id, external_id)
);
CREATE INDEX IF NOT EXISTS idx_task_links_task        ON integration_task_links(task_id);
CREATE INDEX IF NOT EXISTS idx_task_links_integration ON integration_task_links(integration_id);

-- Sync audit log
CREATE TABLE IF NOT EXISTS integration_sync_logs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    integration_id  UUID NOT NULL REFERENCES integrations(id) ON DELETE CASCADE,
    task_link_id    UUID REFERENCES integration_task_links(id) ON DELETE SET NULL,
    direction       VARCHAR(20)  NOT NULL,
    status          VARCHAR(50)  NOT NULL,
    error_message   TEXT,
    request_body    JSONB,
    response_body   JSONB,
    duration_ms     INTEGER,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_sync_logs_integration ON integration_sync_logs(integration_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_sync_logs_link        ON integration_sync_logs(task_link_id);

-- Notification configurations
CREATE TABLE IF NOT EXISTS notification_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    channel_type    VARCHAR(50)  NOT NULL,
    webhook_url     TEXT NOT NULL,
    config          JSONB NOT NULL DEFAULT '{}',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    events          TEXT[] NOT NULL DEFAULT '{}',
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_configs_org ON notification_configs(org_id);
