ALTER TABLE secrets_rotation_runs
    DROP COLUMN IF EXISTS rp_deployments,
    DROP COLUMN IF EXISTS rp_rings,
    DROP COLUMN IF EXISTS rp_app,
    DROP COLUMN IF EXISTS github_secret_names,
    DROP COLUMN IF EXISTS github_environment,
    DROP COLUMN IF EXISTS github_repo,
    DROP COLUMN IF EXISTS k8s_secrets,
    DROP COLUMN IF EXISTS external_secrets,
    DROP COLUMN IF EXISTS cluster,
    DROP COLUMN IF EXISTS source;
