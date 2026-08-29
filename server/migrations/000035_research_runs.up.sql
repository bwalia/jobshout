-- Migration 035: research runs.
--
-- Until now the Research Agent left no trace. research.Agent.Research was a
-- synchronous function that returned a Brief in memory and forgot it, which had
-- four consequences a user could see: the agent could never appear on the agent
-- board (the board unions multi_agent_jobs, blog_runs and mail_threads, and
-- research wrote to none of them); mail_threads.research_brief_id referenced a
-- table that did not exist; the Task Manager held a 180-second browser request
-- and then flattened the findings into the task's description because there was
-- nowhere else to put them; and research done in chat evaporated after the turn.
--
-- This table is the row every research entry point writes, so that all four
-- have somewhere to point.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart, not just this feature.

CREATE TABLE IF NOT EXISTS research_runs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id        UUID REFERENCES agents(id) ON DELETE SET NULL,
    -- The board task this run belongs to, when it was started from one.
    task_id         UUID REFERENCES tasks(id) ON DELETE SET NULL,
    requested_by    UUID REFERENCES users(id) ON DELETE SET NULL,

    -- Which surface asked: task_manager | chat | mail | blog | api. Kept as
    -- free text rather than an enum so a new caller does not need a migration.
    source          VARCHAR(32) NOT NULL DEFAULT 'api',

    topic           TEXT NOT NULL,
    context         TEXT NOT NULL DEFAULT '',
    -- Pinned URLs, when the caller asked for specific pages to be read rather
    -- than an open-web search.
    urls            TEXT[] NOT NULL DEFAULT '{}',

    status          VARCHAR(20) NOT NULL DEFAULT 'queued'
                    CHECK (status IN ('queued','running','completed','failed')),
    -- The live research.ProgressFunc phase (planning/searching/reading/
    -- verifying). Separate from status because the board wants to say what the
    -- agent is doing, not merely that it is busy.
    phase           VARCHAR(20) NOT NULL DEFAULT '',

    -- The whole research.Brief. Stored as JSONB rather than normalised into
    -- findings/sources tables because every consumer reads the brief whole:
    -- normalising it would buy a join and two more migrations and nothing else.
    brief           JSONB,
    -- Mirrors Brief.IsUsable() so "did this produce anything worth using" is
    -- answerable without decoding the JSON.
    usable          BOOLEAN NOT NULL DEFAULT FALSE,
    error_message   TEXT,

    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- The board reads the most recent run per agent; the runs list reads most
-- recent per org. Both are served by this index.
CREATE INDEX IF NOT EXISTS research_runs_org_created_idx
    ON research_runs (org_id, created_at DESC);

CREATE INDEX IF NOT EXISTS research_runs_agent_created_idx
    ON research_runs (agent_id, created_at DESC);

CREATE INDEX IF NOT EXISTS research_runs_task_idx
    ON research_runs (task_id)
    WHERE task_id IS NOT NULL;

-- mail_threads.research_brief_id / mail_drafts.research_brief_id were added in
-- migration 033 naming a table that did not exist, so the value could never be
-- dereferenced: mail_service generated a fresh uuid.New() for it.
--
-- The column is deliberately NOT renamed here. Renaming it would touch about
-- twenty SQL sites in mail_repository.go, two model structs and the
-- research_brief_id field of the mail JSON API, and the thing worth fixing is
-- that the identifier resolves — not what it is called. mail_service now stores
-- the real research_runs.id, so it does.
--
-- No foreign key is added either: rows written before this migration hold
-- invented UUIDs that reference nothing, and a constraint (even NOT VALID)
-- would be a trap for the first person to backfill them.
