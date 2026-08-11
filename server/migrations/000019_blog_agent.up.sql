-- Migration 019: give the article generator an agent identity, a step trace,
-- and somewhere to keep the article body.
--
-- Three changes:
--   1. blog_runs gains agent_id (which agent the run is attributed to), steps
--      (the live progress trace the UI and agent board read), and published_at.
--   2. blog_articles stores the generated markdown. Until now the body was
--      deliberately not persisted ("it lives in the PR"), which made the output
--      unreadable in the product and impossible to review before publishing.
--   3. Every existing organization gets the built-in Article Writer agent.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart, not just this feature.

ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS agent_id     UUID REFERENCES agents(id) ON DELETE SET NULL;
ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS steps        JSONB NOT NULL DEFAULT '[]';
ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS published_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_blog_runs_agent ON blog_runs(agent_id, created_at DESC);

-- Article bodies live in their own table rather than in blog_runs.articles so
-- that listing runs never drags full markdown across the wire, and so each
-- article has a stable id the UI can route to.
CREATE TABLE IF NOT EXISTS blog_articles (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id     UUID NOT NULL REFERENCES blog_runs(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    topic      TEXT NOT NULL,
    slug       VARCHAR(255) NOT NULL,
    path       VARCHAR(500) NOT NULL,
    markdown   TEXT NOT NULL,
    word_count INT  NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_blog_articles_run ON blog_articles(run_id);
CREATE INDEX IF NOT EXISTS idx_blog_articles_org ON blog_articles(org_id, created_at DESC);

-- Backfill: one built-in Article Writer per existing organization.
--
-- agents.metadata (added by 000008) is unused by the rest of the codebase, so
-- it is the natural marker for built-ins — no new column needed. agents.org_id
-- is NOT NULL, so the org_id IS NULL trick used for built-in skills in 000014
-- does not transfer; we insert one row per org instead and guard on the marker.
--
-- Status is 'active' on purpose: the dashboard's Active Agents grid filters on
-- status = 'active', and this agent is always available for work.
--
-- New organizations are seeded by auth_service.Register; this statement covers
-- organizations that already existed when the feature shipped.
INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Article Writer',
    'Content Writer',
    'Writes SEO-optimised technical articles in markdown, then publishes them to the content repository as a pull request for review.',
    'active',
    'go_native',
    'You are a technical blog writer for a developer audience. You produce high-quality, SEO-optimised articles in pure markdown: a single H1 title, H2/H3 structure, 800-1200 words, at least one code block where it helps the reader, and a short Further Reading list.',
    '{"builtin":"article_writer"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'article_writer'
);
