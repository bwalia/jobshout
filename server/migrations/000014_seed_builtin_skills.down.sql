-- Remove the seeded built-in skills. Only the built-ins (org_id IS NULL) with
-- the known slugs are deleted; org-authored skills are never touched. The
-- agent_skills ON DELETE CASCADE cleans up any enablements automatically.
DELETE FROM skills
WHERE org_id IS NULL
  AND slug IN ('web-fetch', 'shell-runner', 'concise-writer', 'cite-sources');
