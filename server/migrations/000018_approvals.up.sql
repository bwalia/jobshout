-- Migration 018: Human-in-the-loop approvals (Phase 3)

-- Pending/decided approvals for gated agent tool calls. When an agent is about
-- to invoke a gated tool the ReAct execution pauses, a row is inserted here with
-- the serialised resume state, and a human approves/rejects out of band; on
-- approval the execution resumes from the stored state.
CREATE TABLE IF NOT EXISTS approvals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    execution_id  UUID NOT NULL REFERENCES agent_executions(id) ON DELETE CASCADE,
    agent_id      UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_name     TEXT NOT NULL,
    tool_input    JSONB NOT NULL DEFAULT '{}',
    status        TEXT NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'approved', 'rejected')),
    reason        TEXT,
    resume_state  JSONB,
    requested_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    decided_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    decided_at    TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS idx_approvals_org_status ON approvals(org_id, status);
CREATE INDEX IF NOT EXISTS idx_approvals_execution  ON approvals(execution_id);

-- Per-agent approval gate: a (agent, tool) pair listed here requires human
-- approval before the agent may execute that tool. A dedicated table is the
-- least-disruptive gate configuration — it leaves agent_tool_permissions
-- untouched and keeps the gate additive and default-off (no row => not gated).
CREATE TABLE IF NOT EXISTS agent_approval_rules (
    agent_id   UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tool_name  TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (agent_id, tool_name)
);
