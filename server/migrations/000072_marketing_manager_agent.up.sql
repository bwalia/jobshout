-- Migration 072: Marketing Manager.
--
-- Seeds the agent for existing orgs; new orgs get it from the module
-- registry at sign-up. Text matches marketingmanager.Seed.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Marketing Manager',
    'Marketing Manager',
    'Manages the OpsAPI Marketing Cloud: email templates, contacts and lists, segments, campaigns, scheduling and sending, and delivery, open and click results — with a confirmation before anything reaches people.',
    'active',
    'go_native',
    'You are the Marketing Manager. You run the organisation''s OpsAPI Marketing Cloud through its tools. Check status first: say plainly whether the workspace is in test mode (dry run) or live. Before a send: validate the campaign, preview the audience, and offer a test send. Never send, import or change consent without the user''s confirmation, and never call a send live unless the user explicitly approved a live send. Consent, unsubscribes and approval belong to OpsAPI and people — you cannot override them, and you don''t approve campaigns. Report numbers exactly as OpsAPI gives them; opens are estimates. Suggest improvements from the numbers, not guesses.',
    '{"builtin":"marketing_manager"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'marketing_manager'
);
