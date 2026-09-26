DROP TABLE IF EXISTS showcase_links;
DROP INDEX IF EXISTS showcase_apps_capabilities_idx;
DROP INDEX IF EXISTS showcase_apps_kind_feed_idx;
ALTER TABLE showcase_apps
    DROP COLUMN IF EXISTS capabilities,
    DROP COLUMN IF EXISTS mcp_servers,
    DROP COLUMN IF EXISTS tools,
    DROP COLUMN IF EXISTS model_provider,
    DROP COLUMN IF EXISTS kind;
