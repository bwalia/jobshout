DROP INDEX IF EXISTS showcase_apps_industries_idx;
ALTER TABLE showcase_apps DROP COLUMN IF EXISTS industries;
DROP TABLE IF EXISTS showcase_industries;
