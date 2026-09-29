DROP TABLE IF EXISTS user_identities;
DROP INDEX IF EXISTS idx_refresh_tokens_device_id;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS device_id;
DROP TABLE IF EXISTS devices;
