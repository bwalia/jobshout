DELETE FROM workflow_step_deps
WHERE step_id IN (
  SELECT s.id FROM workflow_steps s
  JOIN workflows w ON w.id = s.workflow_id
  WHERE w.name = 'Vault SSH then Linux Patch'
);
DELETE FROM workflow_steps WHERE workflow_id IN (SELECT id FROM workflows WHERE name = 'Vault SSH then Linux Patch');
DELETE FROM workflows WHERE name = 'Vault SSH then Linux Patch';
DROP TABLE IF EXISTS linux_patch_runs;
DELETE FROM agents WHERE metadata->>'builtin' = 'linux_patch';
