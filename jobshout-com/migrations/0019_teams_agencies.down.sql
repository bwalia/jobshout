DROP TABLE IF EXISTS analytics_daily;
ALTER TABLE bids DROP COLUMN IF EXISTS team_id;
DROP TABLE IF EXISTS marketplace_team_members;
DROP TABLE IF EXISTS marketplace_teams;
