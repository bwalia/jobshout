-- Migration 050: Linux Patch agent + vault→patch chain workflow seed.
-- IDEMPOTENCY IS MANDATORY.

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Linux Patch',
    'Linux Patch Specialist',
    'SSH rolling Linux patcher with LLM planning. Fetches SSH keys from WSLVault/HashiCorp Vault. Workloads: Couchbase, HC Vault, WSLVault HA. Chain via workflows with Secrets Rotation.',
    'active',
    'go_native',
    'You are the Linux Patch agent. Prefer mode=plan with use_llm. Use vault_path to pull SSH keys from Vault (Secrets Rotation credentials). Orchestrate Secrets Rotation → Linux Patch via workflows. Never print SSH keys.',
    '{"builtin":"linux_patch"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id AND a.metadata->>'builtin' = 'linux_patch'
);

CREATE TABLE IF NOT EXISTS linux_patch_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    mode VARCHAR(20) NOT NULL DEFAULT 'plan'
        CHECK (mode IN ('plan', 'patch', 'verify')),
    workload VARCHAR(40) NOT NULL DEFAULT 'generic',
    hosts TEXT NOT NULL,
    ssh_user TEXT NOT NULL DEFAULT '',
    pre_script TEXT NOT NULL DEFAULT '',
    post_script TEXT NOT NULL DEFAULT '',
    services TEXT NOT NULL DEFAULT '',
    reboot_policy VARCHAR(20) NOT NULL DEFAULT 'if_needed',
    dry_run BOOLEAN NOT NULL DEFAULT true,
    use_llm BOOLEAN NOT NULL DEFAULT true,
    schedule_after TEXT,
    vault_mount TEXT NOT NULL DEFAULT 'secret',
    vault_path TEXT NOT NULL DEFAULT '',
    vault_key_field TEXT NOT NULL DEFAULT 'ssh_private_key',
    instruction TEXT,
    phases JSONB,
    host_results JSONB,
    plan JSONB,
    error_message TEXT,
    requested_by UUID,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_linux_patch_runs_org_id ON linux_patch_runs(org_id);
CREATE INDEX IF NOT EXISTS idx_linux_patch_runs_created_at ON linux_patch_runs(created_at);

-- Seed a chain workflow per org: Secrets Rotation (plan) → Linux Patch (plan).
-- Specialist Launch is used for go_native builtins in the workflow DAG.
INSERT INTO workflows (org_id, name, description, status)
SELECT o.id,
       'Vault SSH then Linux Patch',
       'Orchestration: Secrets Rotation verifies/plans Vault access, then Linux Patch plans a rolling SSH patch using vault_path for keys. Pass input hosts, vault_path, workload.',
       'active'
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM workflows w WHERE w.org_id = o.id AND w.name = 'Vault SSH then Linux Patch'
);

INSERT INTO workflow_steps (workflow_id, name, agent_id, input_template, position, engine_type)
SELECT w.id, 'vault_prepare', a.id,
       E'mode: plan\npath: {{.vault_path}}\nmount: {{.vault_mount}}\nprovider: auto\ninstruction: Prepare Vault access for fleet SSH keys before Linux Patch',
       0, 'go_native'
FROM workflows w
JOIN agents a ON a.org_id = w.org_id AND a.metadata->>'builtin' = 'secrets_rotation'
WHERE w.name = 'Vault SSH then Linux Patch'
  AND NOT EXISTS (
      SELECT 1 FROM workflow_steps s WHERE s.workflow_id = w.id AND s.name = 'vault_prepare'
  );

INSERT INTO workflow_steps (workflow_id, name, agent_id, input_template, position, engine_type)
SELECT w.id, 'linux_patch_plan', a.id,
       E'mode: plan\nworkload: {{.workload}}\nhosts: {{.hosts}}\nvault_path: {{.vault_path}}\nvault_mount: {{.vault_mount}}\ndry_run: true\nuse_llm: true\ninstruction: After Vault prepare — plan zero-downtime patch. Prior step: {{.Outputs.vault_prepare}}',
       1, 'go_native'
FROM workflows w
JOIN agents a ON a.org_id = w.org_id AND a.metadata->>'builtin' = 'linux_patch'
WHERE w.name = 'Vault SSH then Linux Patch'
  AND NOT EXISTS (
      SELECT 1 FROM workflow_steps s WHERE s.workflow_id = w.id AND s.name = 'linux_patch_plan'
  );

INSERT INTO workflow_step_deps (step_id, depends_on_id)
SELECT patch.id, vault.id
FROM workflows w
JOIN workflow_steps vault ON vault.workflow_id = w.id AND vault.name = 'vault_prepare'
JOIN workflow_steps patch ON patch.workflow_id = w.id AND patch.name = 'linux_patch_plan'
WHERE w.name = 'Vault SSH then Linux Patch'
  AND NOT EXISTS (
      SELECT 1 FROM workflow_step_deps d WHERE d.step_id = patch.id AND d.depends_on_id = vault.id
  );
