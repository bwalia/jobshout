-- Reverse of 000020. The pull-request columns come back empty: the runs that
-- once filled them were dropped going up, and nothing since has produced a
-- branch or a PR to restore.

DROP INDEX IF EXISTS idx_blog_articles_post_uuid;

ALTER TABLE blog_articles DROP COLUMN IF EXISTS posted_at;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS post_status;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS post_uuid;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS html;

ALTER TABLE blog_runs DROP COLUMN IF EXISTS cms_namespace;

ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS branch    VARCHAR(255);
ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS pr_number INTEGER;
ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS pr_url    VARCHAR(500);
