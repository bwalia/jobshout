-- Migration 044: WAF Efficacy Lab run / step / result tables.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart. Every object below is created
-- IF NOT EXISTS for that reason.

CREATE TABLE IF NOT EXISTS waf_lab_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    mode VARCHAR(40) NOT NULL DEFAULT 'provision_and_test',
    secure_host TEXT NOT NULL,
    open_host TEXT NOT NULL,
    origin_upstream TEXT NOT NULL DEFAULT '127.0.0.1:30084',
    policy_id TEXT NOT NULL DEFAULT 'waf-policy-payments-hard',
    manage_dns VARCHAR(20) NOT NULL DEFAULT 'off',
    dns_zone TEXT NOT NULL DEFAULT '',
    attack_set VARCHAR(40) NOT NULL DEFAULT 'full',
    instruction TEXT,
    wslproxy_base_url TEXT NOT NULL,
    score JSONB,
    error_message TEXT,
    requested_by UUID,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS waf_lab_steps (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES waf_lab_runs(id) ON DELETE CASCADE,
    phase VARCHAR(40) NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'pending',
    message TEXT NOT NULL DEFAULT '',
    started_at TIMESTAMP,
    ended_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS waf_lab_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES waf_lab_runs(id) ON DELETE CASCADE,
    host_role VARCHAR(20) NOT NULL,
    host TEXT NOT NULL,
    attack_id TEXT NOT NULL,
    attack_name TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL DEFAULT 'GET',
    path TEXT NOT NULL DEFAULT '',
    payload TEXT NOT NULL DEFAULT '',
    status_code INT NOT NULL DEFAULT 0,
    blocked BOOLEAN NOT NULL DEFAULT FALSE,
    expect_block BOOLEAN NOT NULL DEFAULT TRUE,
    waf_rule TEXT NOT NULL DEFAULT '',
    waf_violation TEXT NOT NULL DEFAULT '',
    support_id TEXT NOT NULL DEFAULT '',
    latency_ms INT NOT NULL DEFAULT 0,
    verdict TEXT NOT NULL DEFAULT '',
    notes TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_waf_lab_runs_org_id ON waf_lab_runs(org_id);
CREATE INDEX IF NOT EXISTS idx_waf_lab_runs_agent_id ON waf_lab_runs(agent_id);
CREATE INDEX IF NOT EXISTS idx_waf_lab_runs_created_at ON waf_lab_runs(created_at);
CREATE INDEX IF NOT EXISTS idx_waf_lab_steps_run_id ON waf_lab_steps(run_id);
CREATE INDEX IF NOT EXISTS idx_waf_lab_results_run_id ON waf_lab_results(run_id);
