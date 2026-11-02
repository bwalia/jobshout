-- Migration 074: Meta Lead Analyst → OpsAPI CRM sync columns.
--
-- Stores the OpsAPI CRM contact id after a successful push so re-runs
-- are idempotent and the agent tab can show sync status.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS crm_contact_id VARCHAR(64) NOT NULL DEFAULT '';
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS crm_website VARCHAR(128) NOT NULL DEFAULT '';
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS crm_synced_at TIMESTAMPTZ;
ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS crm_error TEXT NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_meta_leads_crm_contact
    ON meta_leads (org_id, crm_contact_id)
    WHERE crm_contact_id <> '';

UPDATE agents
SET description = 'Pulls Instagram and Facebook Lead Ads Instant Form submissions via the Meta Graph API, runs LLM analysis (per-lead briefs + rollup), and syncs qualified contacts into OpsAPI CRM. New leads can also arrive through the leadgen webhook.',
    updated_at = NOW()
WHERE metadata->>'builtin' = 'meta_lead_analyst'
  AND (description IS DISTINCT FROM 'Pulls Instagram and Facebook Lead Ads Instant Form submissions via the Meta Graph API, runs LLM analysis (per-lead briefs + rollup), and syncs qualified contacts into OpsAPI CRM. New leads can also arrive through the leadgen webhook.');
