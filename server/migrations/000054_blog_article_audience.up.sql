-- Migration 054: record which reader an article was written for.
--
-- The Article Writer can now write for more than one audience — a developer
-- deep dive, a developer technote, a plain-English business briefing — and each
-- is usually its own schedule. That breaks topic de-duplication unless the
-- reader is stored: RecentTopics is what stops a nightly schedule republishing
-- the same story all week, and with one org-wide list a business schedule was
-- locked out of every subject the developer schedule had just covered. Those
-- are two different articles and both are wanted, so "already written about"
-- has to mean "already written about for this reader".
--
-- Nullable with no default on purpose. Every article that existed before this
-- was a developer piece, and empty is how the audience package spells its
-- default — so an old row and a row written with the default chosen are the
-- same row, rather than two shapes meaning the same thing. No backfill for the
-- same reason.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot, so every statement here must be safe to run twice.

ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS audience VARCHAR(64);
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS industry  VARCHAR(255);

-- RecentTopics scans one org's recent articles and now filters by audience.
-- COALESCE(audience, '') is what the query compares, so the index matches that
-- expression rather than the bare column.
CREATE INDEX IF NOT EXISTS idx_blog_articles_org_audience_recent
    ON blog_articles (org_id, COALESCE(audience, ''), created_at DESC);
