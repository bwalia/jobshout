-- Migration 078: what an article's last "Redraw images" did.
-- The page stops waiting long before a redraw on the workstation ends, so its
-- notes (why a figure was dropped, why the cover is a plain diagram) are kept.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS image_redraw JSONB;
