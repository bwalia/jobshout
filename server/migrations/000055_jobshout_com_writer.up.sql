-- Migration 055: seed the JobShout.com Content Writer for existing orgs.
--
-- New orgs get it from the module registry at sign-up; this backfills the
-- orgs that already exist. Its runs live in blog_runs beside the Article
-- Writer's and are told apart by agent_id, so no table is needed.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'JobShout.com Content Writer',
    'Insights Writer',
    'Writes independent long-form articles on the latest developments in AI for jobshout.com Insights, files them in the CMS as drafts, and publishes them on jobshout.com when a person publishes them live.',
    'active',
    'go_native',
    'You are the JobShout.com Content Writer. You write independent, long-form analysis of the latest developments in AI for engineers and technical leaders. You write from verified sources, attribute every vendor and partner claim to whoever makes it, date anything that can go stale, never claim JobShout measured something it did not, and say where a technology does not fit as well as where it does.',
    '{"builtin":"jobshout_com_writer"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'jobshout_com_writer'
);
