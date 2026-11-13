-- Reverse of 0012. Drops new tables/columns; does not restore deleted rows.

UPDATE showcase_follows SET follower_member_id = NULL WHERE follower_member_id IS NOT NULL;
ALTER TABLE showcase_follows DROP COLUMN IF EXISTS follower_member_id;

UPDATE member_follows SET follower_member_id = NULL WHERE follower_member_id IS NOT NULL;
ALTER TABLE member_follows DROP COLUMN IF EXISTS follower_member_id;

ALTER TABLE candidate_profiles DROP COLUMN IF EXISTS account_id;
ALTER TABLE members DROP COLUMN IF EXISTS account_id;

DROP TABLE IF EXISTS email_outbox;
DROP TABLE IF EXISTS notification_preferences;
DROP TABLE IF EXISTS notifications;
DROP TABLE IF EXISTS domain_events;
DROP TABLE IF EXISTS account_roles;
DROP TABLE IF EXISTS account_identities;
DROP TABLE IF EXISTS accounts;
