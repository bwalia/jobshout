DROP TABLE IF EXISTS security_finding_events;
ALTER TABLE waf_lab_runs DROP COLUMN IF EXISTS report_seq;
ALTER TABLE waf_lab_runs DROP COLUMN IF EXISTS app_version;
ALTER TABLE pentest_runs DROP COLUMN IF EXISTS report_seq;
ALTER TABLE pentest_runs DROP COLUMN IF EXISTS app_version;
