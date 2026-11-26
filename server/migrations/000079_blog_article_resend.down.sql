ALTER TABLE blog_articles DROP COLUMN IF EXISTS resend_started_at;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS last_resend;
