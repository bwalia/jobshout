-- Migration 079: "Resend to OpsAPI" for a stored article.
-- last_resend is the outcome of the last resend (time, result, post, error),
-- shown on the article. resend_started_at is the in-flight claim: a resend
-- sets it with a conditional UPDATE and clears it when done, so two resends of
-- one article (double click, two tabs, two API pods) cannot both file a post.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS last_resend JSONB;
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS resend_started_at TIMESTAMPTZ;
