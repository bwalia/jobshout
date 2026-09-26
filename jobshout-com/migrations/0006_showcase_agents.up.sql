-- AI Showcase phase 2: agents and agent teams join apps in showcase_apps
-- (a `kind` discriminator, as Insights does), and showcase_links records who
-- built what: app -> agent, app -> team, team -> member agent.

ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS kind           TEXT   NOT NULL DEFAULT 'app';
ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS model_provider TEXT   NOT NULL DEFAULT '';
ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS tools          TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS mcp_servers    TEXT[] NOT NULL DEFAULT '{}';
ALTER TABLE showcase_apps ADD COLUMN IF NOT EXISTS capabilities   TEXT[] NOT NULL DEFAULT '{}';

CREATE INDEX IF NOT EXISTS showcase_apps_kind_feed_idx
    ON showcase_apps (kind, status, visibility, published_at DESC);
CREATE INDEX IF NOT EXISTS showcase_apps_capabilities_idx
    ON showcase_apps USING GIN (capabilities);

CREATE TABLE IF NOT EXISTS showcase_links (
    from_id   UUID NOT NULL REFERENCES showcase_apps(id) ON DELETE CASCADE,
    to_id     UUID NOT NULL REFERENCES showcase_apps(id) ON DELETE CASCADE,
    role      TEXT NOT NULL DEFAULT '',
    position  INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (from_id, to_id),
    CHECK (from_id <> to_id)
);

CREATE INDEX IF NOT EXISTS showcase_links_to_idx ON showcase_links (to_id);
