-- Migration 003: Task scheduler, session context, multi-level tasks, LLM config management

-- ─── LLM Provider Configurations (user-managed, stored in DB) ───────────────
CREATE TABLE IF NOT EXISTS llm_provider_configs (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(100) NOT NULL,           -- display name e.g. "My Ollama", "Claude Prod"
    provider_type   VARCHAR(50) NOT NULL,             -- ollama, openai, claude
    base_url        TEXT NOT NULL DEFAULT '',
    api_key         TEXT NOT NULL DEFAULT '',          -- encrypted at rest ideally
    default_model   VARCHAR(255) NOT NULL DEFAULT '',
    is_default      BOOLEAN NOT NULL DEFAULT false,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    config_json     JSONB NOT NULL DEFAULT '{}',      -- extra provider-specific config
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_llm_provider_configs_org ON llm_provider_configs(org_id);

-- ─── Scheduled Tasks (cron-based or interval-based agent/workflow execution) ─
CREATE TABLE IF NOT EXISTS scheduled_tasks (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    -- What to run: agent execution or workflow execution
    task_type       VARCHAR(50) NOT NULL DEFAULT 'agent',  -- agent | workflow
    -- Reference to agent or workflow
    agent_id        UUID REFERENCES agents(id) ON DELETE CASCADE,
    workflow_id     UUID REFERENCES workflows(id) ON DELETE CASCADE,
    -- The prompt / input to pass when executing
    input_prompt    TEXT NOT NULL DEFAULT '',
    input_json      JSONB NOT NULL DEFAULT '{}',
    -- LLM override for this scheduled task
    provider_config_id UUID REFERENCES llm_provider_configs(id) ON DELETE SET NULL,
    model_override  VARCHAR(255),
    -- Schedule configuration
    schedule_type   VARCHAR(50) NOT NULL DEFAULT 'cron',  -- cron | interval | once
    cron_expression VARCHAR(100),                          -- "0 */6 * * *"
    interval_seconds INTEGER,                              -- 3600 for every hour
    run_at          TIMESTAMPTZ,                           -- for one-time runs
    -- State
    status          VARCHAR(50) NOT NULL DEFAULT 'active', -- active | paused | completed | failed
    last_run_at     TIMESTAMPTZ,
    next_run_at     TIMESTAMPTZ,
    run_count       INTEGER NOT NULL DEFAULT 0,
    max_runs        INTEGER,                               -- null = unlimited
    -- Retry config
    retry_on_failure BOOLEAN NOT NULL DEFAULT false,
    max_retries     INTEGER NOT NULL DEFAULT 3,
    -- Metadata
    priority        VARCHAR(50) NOT NULL DEFAULT 'medium', -- low | medium | high | critical
    tags            TEXT[] NOT NULL DEFAULT '{}',
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_org ON scheduled_tasks(org_id);
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_status ON scheduled_tasks(status);
CREATE INDEX IF NOT EXISTS idx_scheduled_tasks_next_run ON scheduled_tasks(next_run_at) WHERE status = 'active';

-- Run history for scheduled tasks
CREATE TABLE IF NOT EXISTS scheduled_task_runs (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scheduled_task_id UUID NOT NULL REFERENCES scheduled_tasks(id) ON DELETE CASCADE,
    execution_id      UUID REFERENCES agent_executions(id) ON DELETE SET NULL,
    workflow_run_id   UUID REFERENCES workflow_runs(id) ON DELETE SET NULL,
    status            VARCHAR(50) NOT NULL DEFAULT 'pending',  -- pending | running | completed | failed
    output            TEXT,
    error_message     TEXT,
    started_at        TIMESTAMPTZ,
    completed_at      TIMESTAMPTZ,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_scheduled_task_runs_task ON scheduled_task_runs(scheduled_task_id, created_at DESC);

-- ─── Session Context (preserves conversation context across LLM switches) ────
CREATE TABLE IF NOT EXISTS sessions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    -- Current LLM configuration
    provider_config_id UUID REFERENCES llm_provider_configs(id) ON DELETE SET NULL,
    model_name      VARCHAR(255),
    -- Session state
    status          VARCHAR(50) NOT NULL DEFAULT 'active', -- active | archived | deleted
    -- Context is the accumulated message history (JSON array of messages)
    context_messages JSONB NOT NULL DEFAULT '[]',
    -- Metadata about the session
    total_tokens    INTEGER NOT NULL DEFAULT 0,
    message_count   INTEGER NOT NULL DEFAULT 0,
    -- Tags for organisation
    tags            TEXT[] NOT NULL DEFAULT '{}',
    created_by      UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_sessions_org ON sessions(org_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);

-- Session snapshots: saved checkpoints for context restoration
CREATE TABLE IF NOT EXISTS session_snapshots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id      UUID NOT NULL REFERENCES sessions(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    context_messages JSONB NOT NULL DEFAULT '[]',
    provider_type   VARCHAR(50),
    model_name      VARCHAR(255),
    total_tokens    INTEGER NOT NULL DEFAULT 0,
    message_count   INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_session_snapshots_session ON session_snapshots(session_id, created_at DESC);

-- ─── Multi-level Task Hierarchy ──────────────────────────────────────────────
-- Tasks already support parent_id for nesting. Add a level/depth column and
-- dependency tracking for task-to-task dependencies (beyond parent-child).
CREATE TABLE IF NOT EXISTS task_dependencies (
    task_id         UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    depends_on_id   UUID NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    dependency_type VARCHAR(50) NOT NULL DEFAULT 'blocks', -- blocks | related | subtask
    PRIMARY KEY (task_id, depends_on_id),
    CHECK (task_id != depends_on_id)
);

CREATE INDEX IF NOT EXISTS idx_task_dependencies_task ON task_dependencies(task_id);
CREATE INDEX IF NOT EXISTS idx_task_dependencies_dep ON task_dependencies(depends_on_id);

-- Add depth/level tracking to tasks if not present
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS depth INTEGER NOT NULL DEFAULT 0;
ALTER TABLE tasks ADD COLUMN IF NOT EXISTS tags TEXT[] NOT NULL DEFAULT '{}';
