-- Phase 2b: Scrum-style sprints for agent collaborations.
--
-- A sprint groups a set of multi-agent jobs (and the agents that work on them)
-- into a time-boxed iteration with planning → active → completed lifecycle.

CREATE TABLE IF NOT EXISTS sprints (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    goal        TEXT,
    -- planning | active | completed | cancelled
    status      VARCHAR(50) NOT NULL DEFAULT 'planning',
    start_at    TIMESTAMPTZ,
    end_at      TIMESTAMPTZ,
    -- Velocity: completed_jobs / total_jobs at sprint close. Cached so the
    -- board doesn't recompute on every render.
    velocity    NUMERIC(5,2),
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sprints_org    ON sprints(org_id);
CREATE INDEX IF NOT EXISTS idx_sprints_status ON sprints(org_id, status);

-- Many-to-many between sprints and multi-agent jobs. A job can in principle
-- migrate between sprints (carry-over), so we keep the join table separate
-- rather than adding sprint_id to multi_agent_jobs directly.
CREATE TABLE IF NOT EXISTS sprint_jobs (
    sprint_id    UUID NOT NULL REFERENCES sprints(id) ON DELETE CASCADE,
    job_id       UUID NOT NULL REFERENCES multi_agent_jobs(id) ON DELETE CASCADE,
    -- Ordering within the sprint backlog so the board renders deterministically.
    position     INTEGER NOT NULL DEFAULT 0,
    added_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sprint_id, job_id)
);

CREATE INDEX IF NOT EXISTS idx_sprint_jobs_sprint ON sprint_jobs(sprint_id, position);

-- Agent assignments — who is "on" the sprint. Lets the UI render a list of
-- contributing agents per sprint without scanning every job.
CREATE TABLE IF NOT EXISTS sprint_agents (
    sprint_id   UUID NOT NULL REFERENCES sprints(id) ON DELETE CASCADE,
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    -- planner | executor | reviewer | any (the latter for reserved capacity)
    role_label  VARCHAR(50) NOT NULL DEFAULT 'any',
    added_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sprint_id, agent_id, role_label)
);
