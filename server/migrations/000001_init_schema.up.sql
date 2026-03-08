-- Jobshout initial schema

-- Organizations (created first since users reference it)
CREATE TABLE IF NOT EXISTS organizations (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name        VARCHAR(255) NOT NULL,
    slug        VARCHAR(255) UNIQUE NOT NULL,
    owner_id    UUID,  -- Will be updated after users table is created
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Users
CREATE TABLE IF NOT EXISTS users (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email       VARCHAR(255) UNIQUE NOT NULL,
    password    VARCHAR(255) NOT NULL,
    full_name   VARCHAR(255) NOT NULL,
    avatar_url  VARCHAR(500),
    role        VARCHAR(50) NOT NULL DEFAULT 'member',
    org_id      UUID REFERENCES organizations(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Add the owner foreign key after users table exists
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.table_constraints
        WHERE constraint_name = 'fk_organizations_owner'
    ) THEN
        ALTER TABLE organizations
            ADD CONSTRAINT fk_organizations_owner
            FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE SET NULL;
    END IF;
END $$;

-- Refresh tokens for JWT rotation
CREATE TABLE IF NOT EXISTS refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  VARCHAR(255) NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- AI Agents
CREATE TABLE IF NOT EXISTS agents (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id            UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name              VARCHAR(255) NOT NULL,
    role              VARCHAR(255) NOT NULL DEFAULT '',
    description       TEXT,
    avatar_url        VARCHAR(500),
    status            VARCHAR(50) NOT NULL DEFAULT 'idle',
    model_provider    VARCHAR(100),
    model_name        VARCHAR(100),
    system_prompt     TEXT,
    performance_score DECIMAL(5,2) DEFAULT 0.00,
    manager_id        UUID REFERENCES agents(id) ON DELETE SET NULL,
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Projects
CREATE TABLE IF NOT EXISTS projects (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(50) NOT NULL DEFAULT 'active',
    priority    VARCHAR(50) NOT NULL DEFAULT 'medium',
    owner_id    UUID REFERENCES users(id) ON DELETE SET NULL,
    due_date    TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Project-Agent assignments (many-to-many)
CREATE TABLE IF NOT EXISTS project_agents (
    project_id  UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    role        VARCHAR(100),
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, agent_id)
);

-- Tasks
CREATE TABLE IF NOT EXISTS tasks (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id        UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    parent_id         UUID REFERENCES tasks(id) ON DELETE CASCADE,
    title             VARCHAR(500) NOT NULL,
    description       TEXT,
    status            VARCHAR(50) NOT NULL DEFAULT 'backlog',
    priority          VARCHAR(50) NOT NULL DEFAULT 'medium',
    assigned_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    assigned_user_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    story_points      INTEGER,
    due_date          TIMESTAMPTZ,
    position          INTEGER NOT NULL DEFAULT 0,
    created_by        UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Task labels
CREATE TABLE IF NOT EXISTS task_labels (
    task_id UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    label   VARCHAR(100) NOT NULL,
    color   VARCHAR(20) NOT NULL DEFAULT '#6366f1',
    PRIMARY KEY (task_id, label)
);

-- Task comments / activity log
CREATE TABLE IF NOT EXISTS task_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id    UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    author_id  UUID REFERENCES users(id) ON DELETE SET NULL,
    agent_id   UUID REFERENCES agents(id) ON DELETE SET NULL,
    body       TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Task status history (for metrics)
CREATE TABLE IF NOT EXISTS task_status_history (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id     UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    old_status  VARCHAR(50),
    new_status  VARCHAR(50) NOT NULL,
    changed_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    changed_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Agent knowledge files
CREATE TABLE IF NOT EXISTS knowledge_files (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    filename   VARCHAR(255) NOT NULL,
    content    TEXT NOT NULL DEFAULT '',
    size_bytes INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Marketplace agent templates
CREATE TABLE IF NOT EXISTS marketplace_agents (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name           VARCHAR(255) NOT NULL,
    description    TEXT,
    role           VARCHAR(255),
    category       VARCHAR(100),
    tags           TEXT[],
    avatar_url     VARCHAR(500),
    system_prompt  TEXT,
    model_provider VARCHAR(100),
    model_name     VARCHAR(100),
    downloads      INTEGER NOT NULL DEFAULT 0,
    rating         DECIMAL(3,2) DEFAULT 0.00,
    is_public      BOOLEAN NOT NULL DEFAULT TRUE,
    author_id      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Marketplace bundle contents
CREATE TABLE IF NOT EXISTS marketplace_bundle_agents (
    bundle_id UUID NOT NULL REFERENCES marketplace_agents(id) ON DELETE CASCADE,
    agent_id  UUID NOT NULL REFERENCES marketplace_agents(id) ON DELETE CASCADE,
    PRIMARY KEY (bundle_id, agent_id)
);

-- Agent metrics snapshots
CREATE TABLE IF NOT EXISTS agent_metrics (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    metric_type VARCHAR(100) NOT NULL DEFAULT 'utilization',
    value       DECIMAL(10,4) NOT NULL DEFAULT 0,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- WebSocket event log
CREATE TABLE IF NOT EXISTS ws_events (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_type VARCHAR(100) NOT NULL,
    payload    JSONB NOT NULL,
    org_id     UUID REFERENCES organizations(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Indexes (using IF NOT EXISTS)
CREATE INDEX IF NOT EXISTS idx_users_org_id ON users(org_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_user_id ON refresh_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash ON refresh_tokens(token_hash);
CREATE INDEX IF NOT EXISTS idx_agents_org_id ON agents(org_id);
CREATE INDEX IF NOT EXISTS idx_agents_status ON agents(status);
CREATE INDEX IF NOT EXISTS idx_agents_manager_id ON agents(manager_id);
CREATE INDEX IF NOT EXISTS idx_projects_org_id ON projects(org_id);
CREATE INDEX IF NOT EXISTS idx_projects_status ON projects(status);
CREATE INDEX IF NOT EXISTS idx_tasks_project_id ON tasks(project_id);
CREATE INDEX IF NOT EXISTS idx_tasks_status ON tasks(status);
CREATE INDEX IF NOT EXISTS idx_tasks_assigned_agent ON tasks(assigned_agent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_parent_id ON tasks(parent_id);
CREATE INDEX IF NOT EXISTS idx_tasks_position ON tasks(project_id, status, position);
CREATE INDEX IF NOT EXISTS idx_task_comments_task_id ON task_comments(task_id);
CREATE INDEX IF NOT EXISTS idx_task_status_history_task_id ON task_status_history(task_id);
CREATE INDEX IF NOT EXISTS idx_knowledge_files_agent_id ON knowledge_files(agent_id);
CREATE INDEX IF NOT EXISTS idx_marketplace_agents_category ON marketplace_agents(category);
CREATE INDEX IF NOT EXISTS idx_agent_metrics_agent_id ON agent_metrics(agent_id);
CREATE INDEX IF NOT EXISTS idx_ws_events_org_id ON ws_events(org_id, created_at DESC);
