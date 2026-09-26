-- Migration 045: seed the built-in AB Testing Agent for existing orgs.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot. New organizations are seeded by auth_service.Register (module Seed).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'AB Testing Agent',
    'AB Testing & Canary',
    'Manages wslproxy weighted and canary traffic splits (A/B testing). Drives update_traffic_split / promote / rollback over MCP and observes live assignment — replacement for the standalone abtesting.fictionally.org control UI.',
    'active',
    'go_native',
    'You are the AB Testing Agent. Prefer agent_execute with actions list, set_weights, promote, rollback, or observe. Use wslproxy MCP for live weight changes. Never invent live traffic counts when demo mode is active — say so.',
    '{"builtin":"ab_testing"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'ab_testing'
);
