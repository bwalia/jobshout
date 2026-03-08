-- LLM execution: workflows, steps, agent executions, tool calls

-- Agent tool permissions (which tools each agent is allowed to use)
CREATE TABLE IF NOT EXISTS agent_tool_permissions (
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_name  VARCHAR(100) NOT NULL,
    config     JSONB NOT NULL DEFAULT '{}',
    PRIMARY KEY (agent_id, tool_name)
);

-- Workflows (user-defined multi-agent pipelines)
CREATE TABLE IF NOT EXISTS workflows (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id      UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name        VARCHAR(255) NOT NULL,
    description TEXT,
    status      VARCHAR(50) NOT NULL DEFAULT 'draft',
    created_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Individual steps within a workflow (each step maps to one agent)
CREATE TABLE IF NOT EXISTS workflow_steps (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id     UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    name            VARCHAR(255) NOT NULL,
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    -- JSON template for the task prompt passed to the agent.
    -- Supports {{step.<name>.output}} placeholders for chaining.
    input_template  TEXT NOT NULL DEFAULT '',
    position        INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Step-level dependencies (DAG edges: step depends_on step)
CREATE TABLE IF NOT EXISTS workflow_step_deps (
    step_id        UUID NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    depends_on_id  UUID NOT NULL REFERENCES workflow_steps(id) ON DELETE CASCADE,
    PRIMARY KEY (step_id, depends_on_id)
);

-- Agent execution records (one row per agent task invocation)
CREATE TABLE IF NOT EXISTS agent_executions (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id        UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- Optional: execution triggered by a workflow run
    workflow_run_id UUID,
    step_id         UUID REFERENCES workflow_steps(id) ON DELETE SET NULL,
    -- Input task description handed to the agent
    input_prompt    TEXT NOT NULL,
    -- Final answer produced by the agent
    output          TEXT,
    status          VARCHAR(50) NOT NULL DEFAULT 'pending',
    error_message   TEXT,
    -- Approximate LLM token usage across all iterations
    total_tokens    INTEGER NOT NULL DEFAULT 0,
    -- Number of ReAct loop iterations performed
    iterations      INTEGER NOT NULL DEFAULT 0,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Individual tool calls made during an agent execution
CREATE TABLE IF NOT EXISTS execution_tool_calls (
    id             UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id   UUID NOT NULL REFERENCES agent_executions(id) ON DELETE CASCADE,
    tool_name      VARCHAR(100) NOT NULL,
    input          JSONB NOT NULL DEFAULT '{}',
    output         TEXT,
    error_message  TEXT,
    duration_ms    INTEGER NOT NULL DEFAULT 0,
    called_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Workflow run records (one row per workflow invocation)
CREATE TABLE IF NOT EXISTS workflow_runs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workflow_id  UUID NOT NULL REFERENCES workflows(id) ON DELETE CASCADE,
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status       VARCHAR(50) NOT NULL DEFAULT 'pending',
    input        JSONB NOT NULL DEFAULT '{}',
    -- Accumulated outputs from all steps, keyed by step name
    outputs      JSONB NOT NULL DEFAULT '{}',
    error_message TEXT,
    started_at   TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    triggered_by UUID REFERENCES users(id) ON DELETE SET NULL
);

-- Indexes
CREATE INDEX IF NOT EXISTS idx_agent_tool_permissions_agent ON agent_tool_permissions(agent_id);
CREATE INDEX IF NOT EXISTS idx_workflows_org_id ON workflows(org_id);
CREATE INDEX IF NOT EXISTS idx_workflow_steps_workflow_id ON workflow_steps(workflow_id);
CREATE INDEX IF NOT EXISTS idx_agent_executions_agent_id ON agent_executions(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_executions_status ON agent_executions(status);
CREATE INDEX IF NOT EXISTS idx_agent_executions_run ON agent_executions(workflow_run_id);
CREATE INDEX IF NOT EXISTS idx_execution_tool_calls_exec ON execution_tool_calls(execution_id);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_workflow ON workflow_runs(workflow_id);
CREATE INDEX IF NOT EXISTS idx_workflow_runs_org ON workflow_runs(org_id, created_at DESC);
