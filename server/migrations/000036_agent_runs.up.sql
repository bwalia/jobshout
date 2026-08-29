-- Migration 036: agent runs — one record of "agent X ran with inputs Y".
--
-- "Run this agent" was implemented three times over: a client-side switch in
-- the Task Manager (web/nextjs/lib/agents/launch.ts) posting to a different
-- endpoint per agent, a server-side switch in the chat tools
-- (internal/platformtools/execute.go), and a builtin-unaware generic loop in
-- TaskRunService. They shared no code, so the same agent could behave
-- differently depending on which surface launched it, and four of six run types
-- never reached the agent board because the board reads specialist tables and
-- those runs wrote to different ones.
--
-- This table is what every surface writes. It does not replace the specialist
-- tables — blog_runs, research_runs, pentest_runs, review_runs and task_runs
-- still hold the detail — it names them, so one query can answer "what has this
-- agent been asked to do".
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart, not just this feature.

CREATE TABLE IF NOT EXISTS agent_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id         UUID REFERENCES tasks(id) ON DELETE SET NULL,
    requested_by    UUID REFERENCES users(id) ON DELETE SET NULL,

    -- The platform marker this run was dispatched on. Empty for a user-created
    -- agent taking the generic prompt path.
    builtin         VARCHAR(32) NOT NULL DEFAULT '',
    -- task_manager | chat | api | scheduler. Free text so a new surface needs
    -- no migration.
    source          VARCHAR(32) NOT NULL DEFAULT 'api',

    -- The validated interview result, keyed as agentschema declares. Stored so
    -- a run can be explained or replayed after the fact.
    inputs          JSONB NOT NULL DEFAULT '{}'::jsonb,

    status          VARCHAR(20) NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued','running','completed','failed')),

    -- The specialist row doing the work, and which table it lives in. Text
    -- rather than a foreign key because it points at one of five unrelated
    -- tables, and a column that can reference any of them can reference none
    -- of them in the schema.
    external_run_id TEXT,
    external_kind   VARCHAR(32) NOT NULL DEFAULT '',

    error_message   TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The board reads the most recent run per agent; the runs list reads most
-- recent per org.
CREATE INDEX IF NOT EXISTS agent_runs_agent_created_idx
    ON agent_runs (agent_id, created_at DESC);

CREATE INDEX IF NOT EXISTS agent_runs_org_created_idx
    ON agent_runs (org_id, created_at DESC);

CREATE INDEX IF NOT EXISTS agent_runs_task_idx
    ON agent_runs (task_id)
    WHERE task_id IS NOT NULL;
