-- Migration 049: Secrets Rotation agent (HC Vault + WSLVault) + runs table.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Secrets Rotation',
    'Secrets Rotation Specialist',
    'Rotates secrets on HashiCorp Vault and WSLVault without downtime: KV v2 dual-version windows, transit key rotation, and database lease cutover plans.',
    'active',
    'go_native',
    'You are the Secrets Rotation agent. Prefer mode=plan before rotate. Use Vault-compatible APIs against WSLVault (https://vault.workstation.co.uk, UI https://vault-ui.workstation.co.uk/login, docs https://www.wslvault.org/) or HashiCorp Vault. Never echo secret values — only paths, versions, and keys. For KV v2: write N+1, keep N readable through a grace window, then optionally soft-delete N.',
    '{"builtin":"secrets_rotation"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'secrets_rotation'
);

CREATE TABLE IF NOT EXISTS secrets_rotation_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    mode VARCHAR(20) NOT NULL DEFAULT 'plan'
        CHECK (mode IN ('plan', 'rotate', 'verify', 'rollback')),
    provider VARCHAR(20) NOT NULL DEFAULT 'auto',
    detected_provider TEXT NOT NULL DEFAULT '',
    vault_addr TEXT NOT NULL DEFAULT '',
    mount TEXT NOT NULL DEFAULT 'secret',
    path TEXT NOT NULL,
    engine VARCHAR(20) NOT NULL DEFAULT 'kv2',
    keys TEXT NOT NULL DEFAULT '',
    grace_seconds INT NOT NULL DEFAULT 300,
    retire_old BOOLEAN NOT NULL DEFAULT false,
    dry_run BOOLEAN NOT NULL DEFAULT false,
    instruction TEXT,
    phases JSONB,
    result JSONB,
    error_message TEXT,
    requested_by UUID,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_secrets_rotation_runs_org_id ON secrets_rotation_runs(org_id);
CREATE INDEX IF NOT EXISTS idx_secrets_rotation_runs_created_at ON secrets_rotation_runs(created_at);

INSERT INTO skills (org_id, slug, name, description, kind, config_json, version, status)
SELECT NULL, 'secrets-zero-downtime', 'Zero-downtime secret rotation',
       'KV v2 dual-version windows, transit key rotate, and DB lease cutover for HashiCorp Vault and WSLVault.',
       'prompt',
       '{"prompt":"When rotating secrets: (1) plan first, (2) for KV v2 write version N+1 while N stays readable, (3) hold a dual-read grace window so apps reload, (4) soft-delete N only after verify, never destroy until confirmed. WSLVault UI https://vault-ui.workstation.co.uk/login · API https://vault.workstation.co.uk · docs https://www.wslvault.org/. Never print secret values."}'::jsonb,
       '1.0.0', 'published'
WHERE NOT EXISTS (
    SELECT 1 FROM skills s WHERE s.org_id IS NULL AND s.slug = 'secrets-zero-downtime'
);

INSERT INTO agent_skills (agent_id, skill_id, enabled)
SELECT a.id, s.id, true
FROM agents a
CROSS JOIN skills s
WHERE a.metadata->>'builtin' = 'secrets_rotation'
  AND s.org_id IS NULL
  AND s.slug = 'secrets-zero-downtime'
  AND NOT EXISTS (
      SELECT 1 FROM agent_skills as2
      WHERE as2.agent_id = a.id AND as2.skill_id = s.id
  );
