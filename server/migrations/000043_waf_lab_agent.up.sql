-- Migration 043: seed the built-in WAF Efficacy Lab agent for existing orgs.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot. New organizations are seeded by auth_service.Register (module Seed).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'WAF Efficacy Lab',
    'WAF Efficacy Lab Agent',
    'Provisions a before/after WAF pair on wslproxy (secure vs open) and measures an honest attack matrix — including BOLA gaps and false positives.',
    'active',
    'go_native',
    'You are the WAF Efficacy Lab agent. Drive wslproxy to seed rules, bind policies to vhosts, optionally provision DNS, then fire the attack catalogue through POST /api/waf/test. Report blocked/leaked/false-positive scores honestly. Never claim BOLA is blocked by a signature WAF.',
    '{"builtin":"waf_lab"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'waf_lab'
);
