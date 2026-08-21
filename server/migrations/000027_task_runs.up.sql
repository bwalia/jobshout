-- Migration 027: on-demand task runs (agent execution of a board task).
--
-- A task_run records one "run this task with an agent" invocation launched from
-- the Task Manager. It captures the exact configuration the run used — the
-- resolved prompt, the agent, an optional engine/model override, the extra
-- skills loaded just for this run, free-form key/value inputs, and whether
-- debug was requested — so a run is reproducible and auditable independently of
-- the task, which keeps changing. The heavy execution telemetry (tool calls,
-- per-iteration detail) lives on the linked agent_executions row via
-- execution_id; the summary counts mirrored here (tokens/cost/latency/output)
-- let the runs list render without a join.
--
-- One row per run. task_id is the board task; agent_id is who ran it; org_id is
-- the tenant boundary every read is scoped by. execution_id is filled in once
-- the underlying agent execution record is created.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart, not just this feature. Every
-- object below is created IF NOT EXISTS for that reason.

CREATE TABLE IF NOT EXISTS task_runs (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id        UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    agent_id       UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    org_id         UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- Nullable and ON DELETE SET NULL: the run row outlives its execution record
    -- if executions are ever pruned, and it exists briefly before the execution
    -- row is created.
    execution_id   UUID REFERENCES agent_executions(id) ON DELETE SET NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'queued'
                     CHECK (status IN ('queued','running','completed','failed')),
    prompt         TEXT NOT NULL,
    engine         VARCHAR(30),
    model_provider VARCHAR(50),
    model_name     VARCHAR(100),
    skill_slugs    TEXT[] NOT NULL DEFAULT '{}',
    inputs         JSONB NOT NULL DEFAULT '{}',
    debug          BOOLEAN NOT NULL DEFAULT FALSE,
    output         TEXT,
    error_message  TEXT,
    total_tokens   INT NOT NULL DEFAULT 0,
    cost_usd       DOUBLE PRECISION NOT NULL DEFAULT 0,
    latency_ms     INT NOT NULL DEFAULT 0,
    iterations     INT NOT NULL DEFAULT 0,
    requested_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    started_at     TIMESTAMPTZ,
    completed_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_task_runs_task ON task_runs(task_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_runs_org ON task_runs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_runs_agent ON task_runs(agent_id, created_at DESC);
