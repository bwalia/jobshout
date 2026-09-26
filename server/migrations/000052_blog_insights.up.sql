-- Articles can be filed in JobShout.com Insights as well as the opsapi CMS.
-- The two destinations are tracked separately: an article may be in either,
-- both or neither, and a retry of one must not resend what the other holds.
--
-- Migrations replay on every boot, so every statement here is idempotent.

ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS insights_item_id   VARCHAR(64);
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS insights_slug      VARCHAR(255);
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS insights_status    VARCHAR(50);
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS insights_posted_at TIMESTAMPTZ;

ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS insights_published_at TIMESTAMPTZ;
