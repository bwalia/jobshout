ALTER TABLE showcase_apps DROP COLUMN IF EXISTS revised_at;
ALTER TABLE showcase_apps DROP COLUMN IF EXISTS follower_count;
DROP TABLE IF EXISTS showcase_follows;
DROP TABLE IF EXISTS member_follows;
DROP TABLE IF EXISTS members;
