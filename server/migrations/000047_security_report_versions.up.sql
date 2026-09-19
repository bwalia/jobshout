-- Version stamps on security reports + cross-run finding lifecycle.

ALTER TABLE pentest_runs
  ADD COLUMN IF NOT EXISTS app_version TEXT,
  ADD COLUMN IF NOT EXISTS report_seq INTEGER;

ALTER TABLE waf_lab_runs
  ADD COLUMN IF NOT EXISTS app_version TEXT,
  ADD COLUMN IF NOT EXISTS report_seq INTEGER;

-- One row per detect/fix/still_open event when a run completes.
CREATE TABLE IF NOT EXISTS security_finding_events (
  id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
  agent_kind   TEXT NOT NULL CHECK (agent_kind IN ('pentest', 'waf_lab')),
  subject_key  TEXT NOT NULL,
  finding_key  TEXT NOT NULL,
  event        TEXT NOT NULL CHECK (event IN ('detected', 'still_open', 'fixed')),
  run_id       UUID NOT NULL,
  report_seq   INTEGER,
  app_version  TEXT,
  severity     TEXT,
  title        TEXT NOT NULL DEFAULT '',
  detail       TEXT NOT NULL DEFAULT '',
  created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sec_finding_events_subject
  ON security_finding_events (org_id, agent_kind, subject_key, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_sec_finding_events_run
  ON security_finding_events (run_id);

-- Backfill report_seq for completed pentest runs (per org + target, chronologically).
WITH ordered AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY org_id, lower(trim(target))
           ORDER BY COALESCE(completed_at, created_at), created_at
         ) AS seq
  FROM pentest_runs
  WHERE status IN ('completed', 'budget_exceeded')
)
UPDATE pentest_runs p
SET report_seq = o.seq
FROM ordered o
WHERE p.id = o.id AND p.report_seq IS NULL;

WITH ordered AS (
  SELECT id,
         ROW_NUMBER() OVER (
           PARTITION BY org_id, lower(trim(secure_host))
           ORDER BY COALESCE(completed_at, created_at), created_at
         ) AS seq
  FROM waf_lab_runs
  WHERE status = 'completed'
)
UPDATE waf_lab_runs w
SET report_seq = o.seq
FROM ordered o
WHERE w.id = o.id AND w.report_seq IS NULL;
