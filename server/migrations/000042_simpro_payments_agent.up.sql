-- Migration 042: seed the built-in Simpro Payments and Invoicing Agent for existing orgs.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot. New organizations are seeded by auth_service.Register (module Seed).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Simpro Payments and Invoicing Agent',
    'Simpro Payments & Invoicing',
    'Automates Simpro AR invoicing and payment workflows, and closes the UK F-Gas refrigerant compliance gap with an audit-ready usage ledger. Demo fixtures always work; live reads when SIMPRO_API_KEY is set. Writes discussed with customer — not shipped.',
    'active',
    'go_native',
    'You are the Simpro Payments and Invoicing Agent. You summarise open invoices, aging, payments, and F-Gas refrigerant usage for Simpro Premium builds. Prefer agent_execute with actions summary, invoice_aging, payment_status, reconcile_preview, fgas_report, or month_end. Never invent live Simpro figures when demo mode is active — say so. Do not claim write-back to Simpro is live; it is discussed-not-shipped.',
    '{"builtin":"simpro_payments"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'simpro_payments'
);
