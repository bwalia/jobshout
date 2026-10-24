-- Migration 071: Marketing Email Writer runs.
--
-- One row per email: the brief, the structured draft the model wrote, the HTML
-- rendered from it, and where it was published (an OpsAPI CMS page with
-- template "email", whose public URL is the email's browser version). The
-- agent's tab reads it; the service writes it as the run moves. Re-generating
-- an email updates the same CMS page (upsert by slug), so slug is how a run
-- finds its page again.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

CREATE TABLE IF NOT EXISTS marketing_email_runs (
    id             UUID PRIMARY KEY,
    org_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    task_id        UUID,
    agent_id       UUID,
    requested_by   UUID,
    brief          JSONB NOT NULL DEFAULT '{}'::jsonb,
    subject        TEXT NOT NULL DEFAULT '',
    preheader      TEXT NOT NULL DEFAULT '',
    title          TEXT NOT NULL DEFAULT '',
    slug           TEXT NOT NULL DEFAULT '',
    status         VARCHAR(32) NOT NULL DEFAULT 'writing',
    stage          VARCHAR(32) NOT NULL DEFAULT 'draft',
    message        TEXT NOT NULL DEFAULT '',
    error_message  TEXT NOT NULL DEFAULT '',
    content_html   TEXT NOT NULL DEFAULT '',
    draft_json     JSONB NOT NULL DEFAULT '{}'::jsonb,
    hero_image_url TEXT NOT NULL DEFAULT '',
    page_uuid      TEXT NOT NULL DEFAULT '',
    page_status    VARCHAR(32) NOT NULL DEFAULT '',
    browser_url    TEXT NOT NULL DEFAULT '',
    destination    TEXT NOT NULL DEFAULT '',
    model          TEXT NOT NULL DEFAULT '',
    publish_mode   VARCHAR(16) NOT NULL DEFAULT 'live',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    started_at     TIMESTAMPTZ,
    finished_at    TIMESTAMPTZ,
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_marketing_email_runs_org_created
    ON marketing_email_runs (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_marketing_email_runs_org_slug
    ON marketing_email_runs (org_id, slug) WHERE slug <> '';

-- The OpsAPI Marketing mail-merge template saved from the same email.
ALTER TABLE marketing_email_runs ADD COLUMN IF NOT EXISTS template_uuid TEXT NOT NULL DEFAULT '';
ALTER TABLE marketing_email_runs ADD COLUMN IF NOT EXISTS template_name TEXT NOT NULL DEFAULT '';
ALTER TABLE marketing_email_runs ADD COLUMN IF NOT EXISTS template_outcome VARCHAR(16) NOT NULL DEFAULT '';
ALTER TABLE marketing_email_runs ADD COLUMN IF NOT EXISTS template_message TEXT NOT NULL DEFAULT '';
ALTER TABLE marketing_email_runs ADD COLUMN IF NOT EXISTS template_html TEXT NOT NULL DEFAULT '';
