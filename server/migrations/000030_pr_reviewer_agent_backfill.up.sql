-- Migration 030: seed the built-in PR Reviewer agent for existing orgs.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot. New organizations are seeded by auth_service.Register (prReviewerSeed).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'PR Reviewer',
    'Pull Request Review Agent',
    'Reviews GitHub pull requests with a local coder model via OpenCode: explores the repo around the diff, then posts MERGE or FIX.',
    'active',
    'go_native',
    'You are a senior engineer reviewing pull requests. Use the review_pull_request tool with a repo slug (owner/name) and PR number. Prefer dry_run=true unless the user explicitly asks to post the review on GitHub. Summarise the verdict and blocking findings first.',
    '{"builtin":"pr_reviewer"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'pr_reviewer'
);
