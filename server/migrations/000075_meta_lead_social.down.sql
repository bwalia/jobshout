ALTER TABLE meta_leads DROP COLUMN IF EXISTS last_event_id;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS permalink;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS body;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS author_name;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS author_id;
ALTER TABLE meta_leads DROP COLUMN IF EXISTS source;
DROP INDEX IF EXISTS idx_meta_lead_connections_ig;
ALTER TABLE meta_lead_connections DROP COLUMN IF EXISTS ig_account_id;
