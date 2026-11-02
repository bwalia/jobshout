-- Migration 075: Meta Lead Analyst — DMs, comments and @mentions.
--
-- Organic Facebook and Instagram interactions become meta_leads rows next to
-- Lead Ads form leads. leadgen_id holds the interaction's key: a DM thread
-- ("dm:<account>:<sender>") or the comment / post / media id. last_event_id is
-- the newest message or comment folded in, so a webhook Meta retries is not
-- appended or analysed twice.
--
-- Instagram webhooks name the Instagram professional account, not the Page, so
-- connections carry the Page's linked account id to route them.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

ALTER TABLE meta_lead_connections ADD COLUMN IF NOT EXISTS ig_account_id VARCHAR(64) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_meta_lead_connections_ig
    ON meta_lead_connections (ig_account_id)
    WHERE ig_account_id <> '';

ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS source VARCHAR(32) NOT NULL DEFAULT 'lead_form';
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS author_id VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS author_name VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS body TEXT NOT NULL DEFAULT '';
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS permalink TEXT NOT NULL DEFAULT '';
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS last_event_id TEXT NOT NULL DEFAULT '';

UPDATE agents
SET description = 'Finds sales leads in Instagram and Facebook: Lead Ads Instant Form submissions plus DMs, comments and @mentions arriving by webhook. Each one gets an LLM brief (is it a lead, intent, fit, evidence quote, next action) and qualified contacts sync into OpsAPI CRM.',
    updated_at = NOW()
WHERE metadata->>'builtin' = 'meta_lead_analyst'
  AND (description IS DISTINCT FROM 'Finds sales leads in Instagram and Facebook: Lead Ads Instant Form submissions plus DMs, comments and @mentions arriving by webhook. Each one gets an LLM brief (is it a lead, intent, fit, evidence quote, next action) and qualified contacts sync into OpsAPI CRM.');
