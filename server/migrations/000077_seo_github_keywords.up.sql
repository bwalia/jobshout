-- Migration 077: SEO Analyst — named GitHub connections + multi-page runs.
-- Tokens sealed with SEO_TOKEN_KEY (AES-GCM). Never schema fields.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

CREATE TABLE IF NOT EXISTS seo_github_connections (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name          VARCHAR(128) NOT NULL,
    repo          VARCHAR(255) NOT NULL DEFAULT '', -- owner/repo
    default_branch VARCHAR(128) NOT NULL DEFAULT 'main',
    access_token_enc BYTEA NOT NULL,
    connected_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, name)
);

CREATE INDEX IF NOT EXISTS idx_seo_github_connections_org
    ON seo_github_connections (org_id);

-- Extend seo_runs for URL-list vs GitHub source + progress pages.
ALTER TABLE seo_runs ADD COLUMN IF NOT EXISTS source VARCHAR(20) NOT NULL DEFAULT 'urls';
ALTER TABLE seo_runs ADD COLUMN IF NOT EXISTS github_connection TEXT NOT NULL DEFAULT '';
ALTER TABLE seo_runs ADD COLUMN IF NOT EXISTS path_glob TEXT NOT NULL DEFAULT '';
ALTER TABLE seo_runs ADD COLUMN IF NOT EXISTS max_pages INT NOT NULL DEFAULT 10;
ALTER TABLE seo_runs ADD COLUMN IF NOT EXISTS pages JSONB;
ALTER TABLE seo_runs ADD COLUMN IF NOT EXISTS progress JSONB;

-- url was required for single-page; GitHub runs may use repo from connection.
ALTER TABLE seo_runs ALTER COLUMN url SET DEFAULT '';

-- Refresh agent copy (idempotent description/prompt update via seed-on-register
-- still applies on boot; keep migration skill note current).
INSERT INTO skills (org_id, slug, name, description, kind, config_json, version, status)
SELECT NULL, 'seo-keyword-pages', 'SEO Keyword Pages',
       'Analyse pages one-by-one against a configurable keyword set (URL list or GitHub repo files). Propose title/meta/H1/frontmatter fixes; open a PR when a named GitHub connection has write scope.',
       'prompt',
       '{"prompt":"Drive every recommendation from the operator keyword set. For URL mode, fetch each URL. For GitHub mode, read matching files via the named connection (never ask for a token). Prefer opening a PR branch with concrete markdown/HTML/frontmatter patches; if write fails, return a downloadable suggested diff in the report."}'::jsonb,
       '1.0.0', 'published'
WHERE NOT EXISTS (
    SELECT 1 FROM skills s WHERE s.org_id IS NULL AND s.slug = 'seo-keyword-pages'
);

INSERT INTO agent_skills (agent_id, skill_id, enabled)
SELECT a.id, s.id, true
FROM agents a
CROSS JOIN skills s
WHERE a.metadata->>'builtin' = 'seo_analyst'
  AND s.org_id IS NULL
  AND s.slug = 'seo-keyword-pages'
  AND NOT EXISTS (
      SELECT 1 FROM agent_skills as2
      WHERE as2.agent_id = a.id AND as2.skill_id = s.id
  );
