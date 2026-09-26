DROP INDEX IF EXISTS idx_blog_articles_org_audience_recent;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS industry;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS audience;
