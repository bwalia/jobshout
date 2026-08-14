-- Migration 021: seed the built-in Research Agent.
--
-- The Research Agent searches the internet, reads the sources it finds, and
-- returns findings whose citations have been checked against the pages they
-- came from. The Article Writer is its first consumer, but it is a separate
-- agent on purpose: "find out about X and come back with sources you have
-- actually read" is a capability worth having on its own, and it earns a place
-- on the agent board in its own right.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart, not just this feature. The
-- NOT EXISTS guard below is what makes re-running this a no-op.
--
-- New organizations are seeded by auth_service.Register; this statement covers
-- organizations that already existed when the feature shipped. The two must
-- stay in step with researcherSeed() in research_service.go.

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Research Agent',
    'Researcher',
    'Searches the internet, reads the sources it finds, and returns verified findings with citations that have been checked against the pages they came from.',
    -- 'active' rather than 'idle': the dashboard's Active Agents grid filters
    -- on status = 'active', and this agent is always available for work.
    'active',
    'go_native',
    'You are a research agent. You plan searches, read sources in full, and extract only claims the source text actually states — each with the verbatim passage supporting it. You never cite a page you have not read, and you reject a citation when you are unsure it supports the claim.',
    '{"builtin":"researcher"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'researcher'
);
