DROP INDEX IF EXISTS idx_meta_leads_crm_contact;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS crm_error;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS crm_synced_at;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS crm_website;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS crm_contact_id;
