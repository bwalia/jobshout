-- Migration 061: Company Website Finder.
--
-- Seeds the agent for existing orgs; new orgs get it from the module
-- registry at sign-up.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Company Website Finder',
    'Researcher',
    'Searches the web for a company’s official website from its name and returns the best match with sources.',
    'active',
    'go_native',
    'You find official company websites. You search the web using the company name, prefer the corporate homepage over social or directory pages, never invent a URL, and always report the search evidence you used.',
    '{"builtin":"company_web"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'company_web'
);
