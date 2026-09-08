-- Migration 041: seed the built-in Credit Controller Agent for existing orgs.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot. New organizations are seeded by auth_service.Register (module Seed).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Credit Controller Agent',
    'Credit Controller',
    'Runs the AP credit-controller job: invoice mailbox, AI weekly/monthly sample generation, durable triage, approval queue, and audit.',
    'active',
    'go_native',
    'You are the Credit Controller Agent. You triage supplier invoices, escalate bank-detail and sanctions exceptions, enforce segregation of duties, and never invent PO or bank figures. Prefer agent_execute with a clear action (summary, generate_monthly, triage_untriaged, month_end).',
    '{"builtin":"credit_controller"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'credit_controller'
);
