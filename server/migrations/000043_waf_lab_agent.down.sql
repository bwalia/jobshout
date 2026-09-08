DELETE FROM agents WHERE metadata->>'builtin' = 'waf_lab';
