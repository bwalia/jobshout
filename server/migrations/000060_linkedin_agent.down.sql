DELETE FROM agents WHERE metadata->>'builtin' = 'linkedin_poster';
DROP TABLE IF EXISTS linkedin_posts;
DROP TABLE IF EXISTS linkedin_oauth_states;
DROP TABLE IF EXISTS linkedin_connections;
