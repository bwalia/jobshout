DELETE FROM agent_skills
WHERE skill_id IN (SELECT id FROM skills WHERE org_id IS NULL AND slug = 'seo-site-structure');
DELETE FROM skills WHERE org_id IS NULL AND slug = 'seo-site-structure';
DROP TABLE IF EXISTS seo_runs;
DELETE FROM agents WHERE metadata->>'builtin' = 'seo_analyst';
