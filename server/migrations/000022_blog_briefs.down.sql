-- Reverse of 022.
--
-- topics was left populated alongside briefs precisely so this rollback loses
-- only the context strings and the reference lists, not the record of what each
-- run was asked to write about.

DROP INDEX IF EXISTS idx_blog_articles_org_created;

ALTER TABLE blog_articles DROP COLUMN IF EXISTS references_json;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS title;
ALTER TABLE blog_runs     DROP COLUMN IF EXISTS briefs;
