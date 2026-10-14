-- Migration 069: link generated images to the article run they were drawn for.
--
-- generated_images knew an image was a blog cover or inline figure (source) but
-- not which article, so the Outputs page could only open the picture, never
-- the article it illustrates. The run, not the article row, is the link: the
-- article is saved after its pictures are drawn, and the web article page is
-- /articles/<run id>.
--
-- Nullable and SET NULL: most images (agent tool, manual) have no run, and
-- deleting a run should not delete the pictures it paid for.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

ALTER TABLE generated_images
    ADD COLUMN IF NOT EXISTS blog_run_id UUID REFERENCES blog_runs(id) ON DELETE SET NULL;

-- Backfill covers drawn before this column existed. A cover's URL is stored on
-- its article, so the match is exact. Linked rows are skipped, so a replay
-- changes nothing.
UPDATE generated_images gi
   SET blog_run_id = ba.run_id
  FROM blog_articles ba
 WHERE gi.blog_run_id IS NULL
   AND gi.url IS NOT NULL
   AND gi.url = ba.cover_image_url
   AND gi.org_id = ba.org_id;
