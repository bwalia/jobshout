DELETE FROM agents WHERE metadata->>'builtin' = 'meta_lead_analyst';
DROP TABLE IF EXISTS meta_leads;
DROP TABLE IF EXISTS meta_lead_runs;
DROP TABLE IF EXISTS meta_lead_connections;
