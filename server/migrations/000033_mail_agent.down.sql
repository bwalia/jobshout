DROP TABLE IF EXISTS mail_drafts;
DROP TABLE IF EXISTS mail_threads;
DROP TABLE IF EXISTS mail_oauth_states;
DROP TABLE IF EXISTS mail_connections;
DELETE FROM agents WHERE metadata->>'builtin' = 'mail';
