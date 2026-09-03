DROP TABLE IF EXISTS auth_oauth_tickets;
DROP TABLE IF EXISTS auth_oauth_states;
DROP INDEX IF EXISTS users_google_sub_uidx;
ALTER TABLE users DROP COLUMN IF EXISTS google_sub;
