-- Migration 004: Multi-runtime execution engine support
-- Adds engine_type to agents, executions, and workflow steps.
-- Creates trace tables for LangChain and LangGraph observability.

-- Engine type on agents: controls which runtime executes this agent.
ALTER TABLE agents ADD COLUMN IF NOT EXISTS engine_type VARCHAR(50) NOT NULL DEFAULT 'go_native';
ALTER TABLE agents ADD COLUMN IF NOT EXISTS engine_config JSONB NOT NULL DEFAULT '{}';
CREATE INDEX IF NOT EXISTS idx_agents_engine_type ON agents(engine_type);

-- Engine type on executions: audit trail of which engine ran.
ALTER TABLE agent_executions ADD COLUMN IF NOT EXISTS engine_type VARCHAR(50) NOT NULL DEFAULT 'go_native';

-- Engine type on workflow steps: per-step engine override (NULL = inherit from agent).
ALTER TABLE workflow_steps ADD COLUMN IF NOT EXISTS engine_type VARCHAR(50);

-- LangChain run traces: per-execution chain step traces.
CREATE TABLE IF NOT EXISTS langchain_run_traces (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL REFERENCES agent_executions(id) ON DELETE CASCADE,
    run_id          VARCHAR(255) NOT NULL,
    chain_type      VARCHAR(100) NOT NULL,
    input_text      TEXT,
    output_text     TEXT,
    error           TEXT,
    latency_ms      INTEGER NOT NULL DEFAULT 0,
    total_tokens    INTEGER NOT NULL DEFAULT 0,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_langchain_traces_exec ON langchain_run_traces(execution_id);

-- LangGraph state snapshots: captures graph state after each node execution.
CREATE TABLE IF NOT EXISTS langgraph_state_snapshots (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    execution_id    UUID NOT NULL REFERENCES agent_executions(id) ON DELETE CASCADE,
    step_number     INTEGER NOT NULL DEFAULT 0,
    node_name       VARCHAR(255) NOT NULL,
    state_json      JSONB NOT NULL DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_langgraph_snapshots_exec ON langgraph_state_snapshots(execution_id, step_number);
