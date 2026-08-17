-- Reverse of 023.
--
-- Dropping generated_images discards the record of what was drawn; the images
-- themselves live in object storage and are not touched here, so a rollback
-- loses the index of them rather than the files.

DROP INDEX IF EXISTS idx_generated_images_org_created;
DROP TABLE IF EXISTS generated_images;

ALTER TABLE blog_articles DROP COLUMN IF EXISTS cover_image_meta;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS cover_image_prompt;
ALTER TABLE blog_articles DROP COLUMN IF EXISTS cover_image_url;
