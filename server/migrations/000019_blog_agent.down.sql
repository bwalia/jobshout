-- Reverse of 000019. The built-in agents are removed by their metadata marker;
-- any agent a user created by hand is left alone.

DELETE FROM agents WHERE metadata->>'builtin' = 'article_writer';

DROP TABLE IF EXISTS blog_articles;

DROP INDEX IF EXISTS idx_blog_runs_agent;

ALTER TABLE blog_runs DROP COLUMN IF EXISTS published_at;
ALTER TABLE blog_runs DROP COLUMN IF EXISTS steps;
ALTER TABLE blog_runs DROP COLUMN IF EXISTS agent_id;
