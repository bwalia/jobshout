-- 000009_autonomous_agents_telegram
-- Adds: autonomous agent loop (goals, plans, memory), Telegram integration,
-- chat sessions, multi-agent collaboration, and audit/rate-limiting tables.

-- ── Agent Memory: Short-Term (per-session conversation buffer) ──────────────
CREATE TABLE IF NOT EXISTS agent_memory_short_term (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    session_id UUID NOT NULL,
    messages   JSONB NOT NULL DEFAULT '[]',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_memory_st_agent_session
    ON agent_memory_short_term (agent_id, session_id);

-- ── Agent Memory: Long-Term (searchable knowledge base) ─────────────────────
CREATE TABLE IF NOT EXISTS agent_memory_long_term (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    content    TEXT NOT NULL,
    summary    TEXT NOT NULL DEFAULT '',
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_memory_lt_agent ON agent_memory_long_term (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_memory_lt_org   ON agent_memory_long_term (org_id);

-- ── Agent Goals (Goal → Plan → Act → Observe → Reflect lifecycle) ───────────
CREATE TABLE IF NOT EXISTS agent_goals (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id     UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    session_id   UUID,
    goal_text    TEXT NOT NULL,
    plan         JSONB NOT NULL DEFAULT '[]',
    status       TEXT NOT NULL DEFAULT 'pending'
                     CHECK (status IN ('pending','planning','executing','reflecting','completed','failed')),
    reflection   TEXT,
    iterations   INT NOT NULL DEFAULT 0,
    max_iter     INT NOT NULL DEFAULT 5,
    error_msg    TEXT,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_agent_goals_agent  ON agent_goals (agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_goals_org    ON agent_goals (org_id);
CREATE INDEX IF NOT EXISTS idx_agent_goals_status ON agent_goals (status);

-- ── Multi-Agent Jobs (planner → executor → reviewer collaboration) ──────────
CREATE TABLE IF NOT EXISTS multi_agent_jobs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    task_prompt   TEXT NOT NULL,
    planner_id    UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    executor_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    reviewer_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    status        TEXT NOT NULL DEFAULT 'pending'
                      CHECK (status IN ('pending','planning','executing','reviewing','completed','failed')),
    plan_output   TEXT,
    exec_output   TEXT,
    review_output TEXT,
    approved      BOOLEAN,
    iterations    INT NOT NULL DEFAULT 0,
    max_review    INT NOT NULL DEFAULT 2,
    error_msg     TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    completed_at  TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_multi_agent_jobs_org    ON multi_agent_jobs (org_id);
CREATE INDEX IF NOT EXISTS idx_multi_agent_jobs_status ON multi_agent_jobs (status);

-- ── Chat Sessions ───────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS chat_sessions (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    agent_id   UUID REFERENCES agents(id) ON DELETE SET NULL,
    source     TEXT NOT NULL DEFAULT 'web' CHECK (source IN ('web','telegram','api')),
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_sessions_org_user ON chat_sessions (org_id, user_id);
CREATE INDEX IF NOT EXISTS idx_chat_sessions_source   ON chat_sessions (source);

-- ── Chat Messages (audit trail for every turn) ─────────────────────────────
CREATE TABLE IF NOT EXISTS chat_messages (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL REFERENCES chat_sessions(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    role       TEXT NOT NULL CHECK (role IN ('user','agent','system')),
    source     TEXT NOT NULL DEFAULT 'web' CHECK (source IN ('web','telegram','api')),
    content    TEXT NOT NULL,
    metadata   JSONB NOT NULL DEFAULT '{}',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_chat_messages_session ON chat_messages (session_id, created_at);
CREATE INDEX IF NOT EXISTS idx_chat_messages_org     ON chat_messages (org_id);

-- ── Telegram User Mappings ──────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS telegram_user_mappings (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    telegram_user_id  BIGINT NOT NULL,
    telegram_username TEXT NOT NULL DEFAULT '',
    jobshout_user_id  UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id            UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    verified          BOOLEAN NOT NULL DEFAULT FALSE,
    linked_at         TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_telegram_mapping_tg_user
    ON telegram_user_mappings (telegram_user_id);
CREATE INDEX IF NOT EXISTS idx_telegram_mapping_js_user
    ON telegram_user_mappings (jobshout_user_id);

-- ── Telegram Link Tokens (one-time account linking) ─────────────────────────
CREATE TABLE IF NOT EXISTS telegram_link_tokens (
    token      TEXT PRIMARY KEY,
    user_id    UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    org_id     UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_telegram_link_tokens_user ON telegram_link_tokens (user_id);

-- ── Telegram Rate Limits (token bucket per user) ────────────────────────────
CREATE TABLE IF NOT EXISTS telegram_rate_limits (
    telegram_user_id  BIGINT PRIMARY KEY,
    tokens_remaining  NUMERIC NOT NULL DEFAULT 20,
    last_refill       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
