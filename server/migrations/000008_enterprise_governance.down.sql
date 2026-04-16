-- 000008_enterprise_governance.down.sql

ALTER TABLE organizations DROP COLUMN IF EXISTS metadata;
ALTER TABLE agents        DROP COLUMN IF EXISTS metadata;
ALTER TABLE tasks         DROP COLUMN IF EXISTS metadata;

DROP TABLE IF EXISTS agent_scores;
DROP TABLE IF EXISTS audit_logs;
DROP TABLE IF EXISTS cost_records;
DROP TABLE IF EXISTS usage_records;
DROP TABLE IF EXISTS pricing_configs;
DROP TABLE IF EXISTS login_audit_logs;
DROP TABLE IF EXISTS sso_configs;
DROP TABLE IF EXISTS user_roles;
DROP TABLE IF EXISTS roles;
