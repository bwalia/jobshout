DROP TABLE IF EXISTS facebook_posts;
DROP TABLE IF EXISTS facebook_oauth_states;
DROP TABLE IF EXISTS facebook_pages;
DROP TABLE IF EXISTS facebook_connections;
DELETE FROM agents WHERE metadata->>'builtin' = 'facebook_page_manager';
