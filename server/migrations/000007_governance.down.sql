-- 000007_governance.down.sql

DROP TABLE IF EXISTS budget_alerts;
DROP TABLE IF EXISTS usage_rollups;
DROP TABLE IF EXISTS agent_policies;
DROP TABLE IF EXISTS org_budgets;

DROP INDEX IF EXISTS idx_agent_executions_org_created;
DROP INDEX IF EXISTS idx_agent_executions_agent_created;

ALTER TABLE agent_executions
    DROP COLUMN IF EXISTS input_tokens,
    DROP COLUMN IF EXISTS output_tokens,
    DROP COLUMN IF EXISTS latency_ms,
    DROP COLUMN IF EXISTS cost_usd,
    DROP COLUMN IF EXISTS model_name,
    DROP COLUMN IF EXISTS model_provider;
