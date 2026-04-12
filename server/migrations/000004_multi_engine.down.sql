-- Rollback migration 004: Multi-runtime execution engine support

DROP TABLE IF EXISTS langgraph_state_snapshots;
DROP TABLE IF EXISTS langchain_run_traces;
ALTER TABLE workflow_steps DROP COLUMN IF EXISTS engine_type;
ALTER TABLE agent_executions DROP COLUMN IF EXISTS engine_type;
ALTER TABLE agents DROP COLUMN IF EXISTS engine_config;
ALTER TABLE agents DROP COLUMN IF EXISTS engine_type;
