-- Reverse of 021: remove the built-in Research Agent.
--
-- Scoped to the builtin marker so an agent a user created and named "Research
-- Agent" themselves is left alone. Runs attributed to it keep their rows —
-- blog_runs.agent_id is ON DELETE SET NULL — so history survives the rollback.

DELETE FROM agents WHERE metadata->>'builtin' = 'researcher';
