-- Migration 031: PR Reviewer now posts to GitHub by default.
--
-- Existing orgs were seeded (migration 030 / prReviewerSeed) with a prompt that
-- told the agent to "Prefer dry_run=true". Update those rows to the post-by-default
-- guidance. IDEMPOTENT: the WHERE clause matches only the old wording, so replays
-- after the first are no-ops (database/migrate.go replays every *.up.sql on boot).

UPDATE agents
SET system_prompt = 'You are a senior engineer reviewing pull requests. Use the review_pull_request tool with a repo slug (owner/name) and PR number. It posts the review to the PR by default; pass dry_run=true only if the user explicitly asks for a preview that posts nothing. Summarise the verdict and blocking findings first.'
WHERE metadata->>'builtin' = 'pr_reviewer'
  AND system_prompt LIKE '%Prefer dry_run=true%';
