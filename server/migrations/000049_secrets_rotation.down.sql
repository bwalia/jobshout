DELETE FROM agent_skills
WHERE skill_id IN (SELECT id FROM skills WHERE org_id IS NULL AND slug = 'secrets-zero-downtime');
DELETE FROM skills WHERE org_id IS NULL AND slug = 'secrets-zero-downtime';
DROP TABLE IF EXISTS secrets_rotation_runs;
DELETE FROM agents WHERE metadata->>'builtin' = 'secrets_rotation';
