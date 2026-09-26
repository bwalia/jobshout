-- Migration 048: SEO Analyst agent seed + run table.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'SEO Analyst',
    'SEO Analyst & Manager',
    'Analyses SEO for a URL (title, meta, headings, sitemap, robots), proposes slug/meta/sitemap fixes, and publishes via WordPress, OpsAPI, or a git PR when a repo is provided.',
    'active',
    'go_native',
    'You are the SEO Analyst agent. Crawl the given URL, score SEO health, propose concrete fixes (meta tags, URL slug, sitemap.xml), and when asked to publish use WordPress REST, OpsAPI CMS, or open a GitHub PR with sitemap + SEO_PATCH.md. Prefer API publish; fall back to git PR when APIs fail or the site is static.',
    '{"builtin":"seo_analyst"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'seo_analyst'
);

CREATE TABLE IF NOT EXISTS seo_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    mode VARCHAR(20) NOT NULL DEFAULT 'analyze'
        CHECK (mode IN ('analyze', 'improve', 'publish')),
    url TEXT NOT NULL,
    platform VARCHAR(20) NOT NULL DEFAULT 'auto',
    detected_cms TEXT NOT NULL DEFAULT '',
    git_repo TEXT NOT NULL DEFAULT '',
    git_branch TEXT NOT NULL DEFAULT 'main',
    keywords TEXT NOT NULL DEFAULT '',
    instruction TEXT,
    score JSONB,
    report JSONB,
    publish JSONB,
    error_message TEXT,
    requested_by UUID,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_seo_runs_org_id ON seo_runs(org_id);
CREATE INDEX IF NOT EXISTS idx_seo_runs_created_at ON seo_runs(created_at);

-- Built-in prompt skill: locate site structure from README / STRUCTURE.md / common CMS layouts.
INSERT INTO skills (org_id, slug, name, description, kind, config_json, version, status)
SELECT NULL, 'seo-site-structure', 'SEO Site Structure',
       'Locate content/public roots and sitemap paths from README.md, STRUCTURE.md, and common CMS/static layouts (WordPress, OpsAPI, Hugo, Jekyll, Next.js).',
       'prompt',
       '{"prompt":"When improving SEO for a git-backed site, locate structure via README.md, STRUCTURE.md, or docs/seo.md. Prefer public/, dist/, docs/, _site/ for HTML output and content/, src/pages/, app/, _posts/ for sources. WordPress: wp-content/themes and /wp-sitemap.xml. OpsAPI: seo_title/seo_description on posts. Next.js: public/sitemap.xml and metadata in layout. Place sitemap.xml under the public root; write SEO_PATCH.md with meta/slug proposals for humans and other agents."}'::jsonb,
       '1.0.0', 'published'
WHERE NOT EXISTS (
    SELECT 1 FROM skills s WHERE s.org_id IS NULL AND s.slug = 'seo-site-structure'
);

-- Enable the skill on every SEO Analyst agent (idempotent).
INSERT INTO agent_skills (agent_id, skill_id, enabled)
SELECT a.id, s.id, true
FROM agents a
CROSS JOIN skills s
WHERE a.metadata->>'builtin' = 'seo_analyst'
  AND s.org_id IS NULL
  AND s.slug = 'seo-site-structure'
  AND NOT EXISTS (
      SELECT 1 FROM agent_skills as2
      WHERE as2.agent_id = a.id AND as2.skill_id = s.id
  );
