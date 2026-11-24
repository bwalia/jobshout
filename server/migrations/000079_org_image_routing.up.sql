-- Migration 079: per-org image model per task.
--
-- routes maps an image task (generated_images.source: blog_cover, course, …)
-- to the provider and model that draw it:
--   {"blog_cover": {"provider": "gemini", "model": "gemini-3-pro-image"}}
-- A task without an entry, or whose provider is no longer configured, uses
-- the platform default.
CREATE TABLE IF NOT EXISTS org_image_routing (
    org_id      UUID PRIMARY KEY REFERENCES organizations(id) ON DELETE CASCADE,
    routes      JSONB NOT NULL DEFAULT '{}',
    updated_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
