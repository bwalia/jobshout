DROP TABLE IF EXISTS meta_lead_evals;
DROP INDEX IF EXISTS idx_meta_leads_labelled;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS label_is_lead;
