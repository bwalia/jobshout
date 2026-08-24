DELETE FROM agents WHERE metadata->>'builtin' = 'pr_reviewer';
