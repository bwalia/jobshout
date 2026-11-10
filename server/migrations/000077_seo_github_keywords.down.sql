DROP TABLE IF EXISTS seo_github_connections;

ALTER TABLE seo_runs DROP COLUMN IF EXISTS source;
ALTER TABLE seo_runs DROP COLUMN IF EXISTS github_connection;
ALTER TABLE seo_runs DROP COLUMN IF EXISTS path_glob;
ALTER TABLE seo_runs DROP COLUMN IF EXISTS max_pages;
ALTER TABLE seo_runs DROP COLUMN IF EXISTS pages;
ALTER TABLE seo_runs DROP COLUMN IF EXISTS progress;

DELETE FROM agent_skills
WHERE skill_id IN (SELECT id FROM skills WHERE org_id IS NULL AND slug = 'seo-keyword-pages');
DELETE FROM skills WHERE org_id IS NULL AND slug = 'seo-keyword-pages';
