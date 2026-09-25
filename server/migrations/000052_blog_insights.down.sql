ALTER TABLE blog_runs DROP COLUMN IF EXISTS insights_published_at;

ALTER TABLE blog_articles DROP COLUMN IF EXISTS insights_posted_at;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS insights_status;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS insights_slug;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS insights_item_id;
