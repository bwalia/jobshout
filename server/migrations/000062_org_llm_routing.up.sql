-- Migration 062: per-org model routing.
--
-- An org with active llm_provider_configs has its model calls routed through
-- them instead of the server's env providers. This row is optional: without
-- it the chain is every active config of the org (is_default first). With it
-- the org names an ORDERED fallback chain of configs (each optionally with its
-- own model), or turns org routing off (enabled = false) to use the env
-- default while keeping its configs.
--
-- chain: [{"provider_config_id": "<uuid>", "model": "<optional model>"}, ...]
CREATE TABLE IF NOT EXISTS org_llm_routing (
    org_id      UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    enabled     BOOLEAN NOT NULL DEFAULT true,
    chain       JSONB NOT NULL DEFAULT '[]',
    updated_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
