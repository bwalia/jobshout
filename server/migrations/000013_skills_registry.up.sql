-- Phase 4: OpenClaw-inspired "skills" registry.
--
-- A skill is a named bundle of capabilities (a tool pack, prompt, or both)
-- that can be enabled per agent. This is the lightest viable shape: a
-- catalog table + a join table for per-agent enablement. We do NOT yet
-- ship an execution path that calls a skill — that is intentionally left
-- to a later phase so this migration is reversible without orphaning data.

CREATE TABLE IF NOT EXISTS skills (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    -- NULL org_id means "built-in / community" — visible to every org.
    -- Non-null means an org-private skill authored locally.
    org_id       UUID REFERENCES organizations(id) ON DELETE CASCADE,
    slug         VARCHAR(120) NOT NULL,
    name         VARCHAR(255) NOT NULL,
    description  TEXT,
    -- "tool" (a callable capability), "prompt" (a system-prompt patch),
    -- or "bundle" (a named group of other skills). Free-form on purpose so
    -- new kinds can land without a schema change.
    kind         VARCHAR(50) NOT NULL DEFAULT 'tool',
    -- Free-form configuration the future executor will consume (e.g. tool
    -- name, allowed args, prompt fragment).
    config_json  JSONB NOT NULL DEFAULT '{}',
    version      VARCHAR(50) NOT NULL DEFAULT '0.1.0',
    -- "draft" | "published" | "deprecated" — published skills appear in the
    -- enable picker, draft does not.
    status       VARCHAR(50) NOT NULL DEFAULT 'published',
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_skills_org_status ON skills(org_id, status);

-- Per-agent skill enablement. An agent can have a skill enabled with a
-- per-agent config override (e.g. a different default arg).
CREATE TABLE IF NOT EXISTS agent_skills (
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    skill_id        UUID NOT NULL REFERENCES skills(id) ON DELETE CASCADE,
    config_override JSONB NOT NULL DEFAULT '{}',
    enabled         BOOLEAN NOT NULL DEFAULT true,
    enabled_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (agent_id, skill_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_skills_agent ON agent_skills(agent_id) WHERE enabled;
