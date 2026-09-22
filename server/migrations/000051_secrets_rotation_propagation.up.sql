-- Migration 051: Secrets Rotation propagation targets.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).
-- Names only (cluster = kubeconfig name, namespace/name lists, repo, rings).
-- Credentials are server-side env/mounts and never stored here.

ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'vault';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS cluster TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS external_secrets TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS k8s_secrets TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS github_repo TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS github_environment TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS github_secret_names TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS rp_app TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS rp_rings TEXT NOT NULL DEFAULT '';
ALTER TABLE secrets_rotation_runs ADD COLUMN IF NOT EXISTS rp_deployments TEXT NOT NULL DEFAULT '';
