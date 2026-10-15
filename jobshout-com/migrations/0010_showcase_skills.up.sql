-- Agent skills catalog with proficiency levels (vision: bars on agent profiles,
-- browse-by-skill in the directory). Distinct from free-text technologies on
-- apps and from the fixed AGENT_CAPABILITIES enum.

CREATE TABLE IF NOT EXISTS showcase_skills (
    slug         TEXT PRIMARY KEY,
    name         TEXT NOT NULL,
    category     TEXT NOT NULL DEFAULT 'general',
    description  TEXT NOT NULL DEFAULT '',
    position     INT  NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS showcase_agent_skills (
    agent_id   UUID NOT NULL REFERENCES showcase_apps(id) ON DELETE CASCADE,
    skill_slug TEXT NOT NULL REFERENCES showcase_skills(slug) ON DELETE CASCADE,
    level      INT  NOT NULL CHECK (level BETWEEN 1 AND 10),
    position   INT  NOT NULL DEFAULT 0,
    PRIMARY KEY (agent_id, skill_slug)
);

CREATE INDEX IF NOT EXISTS showcase_agent_skills_skill_idx
    ON showcase_agent_skills (skill_slug);

-- Curated starters; creators can still add new skills on submit (upsert).
INSERT INTO showcase_skills (slug, name, category, description, position) VALUES
    ('rust', 'Rust', 'language', '', 1),
    ('python', 'Python', 'language', '', 2),
    ('typescript', 'TypeScript', 'language', '', 3),
    ('go', 'Go', 'language', '', 4),
    ('sql', 'SQL', 'language', '', 5),
    ('axum', 'Axum', 'framework', '', 10),
    ('tokio', 'Tokio', 'framework', '', 11),
    ('sqlx', 'SQLx', 'framework', '', 12),
    ('next-js', 'Next.js', 'framework', '', 13),
    ('react', 'React', 'framework', '', 14),
    ('apis', 'APIs', 'domain', 'HTTP APIs, contracts and integration.', 20),
    ('system-design', 'System design', 'domain', '', 21),
    ('kubernetes', 'Kubernetes', 'platform', '', 30),
    ('docker', 'Docker', 'platform', '', 31),
    ('postgresql', 'PostgreSQL', 'platform', '', 32),
    ('security', 'Security', 'domain', 'Application and platform security.', 40),
    ('application-security', 'Application security', 'domain', '', 41),
    ('testing', 'Testing', 'domain', '', 42),
    ('technical-writing', 'Technical writing', 'domain', '', 50),
    ('openapi', 'OpenAPI', 'domain', '', 51),
    ('fhir', 'FHIR', 'domain', '', 60),
    ('kyc', 'KYC', 'domain', '', 61),
    ('aml', 'AML', 'domain', '', 62)
ON CONFLICT (slug) DO NOTHING;

-- Backfill: each agent technology becomes a skill at level 8 (solid, not claimed max).
INSERT INTO showcase_skills (slug, name, category, description, position)
SELECT DISTINCT
    lower(regexp_replace(regexp_replace(trim(t), '[^a-zA-Z0-9]+', '-', 'g'), '(^-|-$)', '', 'g')),
    trim(t),
    'general',
    '',
    100
FROM showcase_apps a, unnest(a.technologies) t
WHERE a.kind = 'agent'
  AND trim(t) <> ''
  AND lower(regexp_replace(regexp_replace(trim(t), '[^a-zA-Z0-9]+', '-', 'g'), '(^-|-$)', '', 'g')) <> ''
ON CONFLICT (slug) DO NOTHING;

INSERT INTO showcase_agent_skills (agent_id, skill_slug, level, position)
SELECT a.id,
       lower(regexp_replace(regexp_replace(trim(t), '[^a-zA-Z0-9]+', '-', 'g'), '(^-|-$)', '', 'g')),
       8,
       ord.ordinality::int - 1
FROM showcase_apps a
CROSS JOIN LATERAL unnest(a.technologies) WITH ORDINALITY AS ord(t, ordinality)
WHERE a.kind = 'agent'
  AND trim(t) <> ''
  AND lower(regexp_replace(regexp_replace(trim(t), '[^a-zA-Z0-9]+', '-', 'g'), '(^-|-$)', '', 'g')) <> ''
ON CONFLICT (agent_id, skill_slug) DO NOTHING;
