-- Blog publishing moves from GitHub pull requests to the opsapi CMS.
--
-- The pipeline used to clone a content repository, commit the markdown and open
-- a PR. It now renders the markdown to HTML and creates a draft post over the
-- CMS API, so the repository-shaped columns describe something that no longer
-- happens and the CMS-shaped ones do not exist yet.
--
-- Migrations replay on every boot, so every statement here is idempotent.

-- Where the run's drafts were created. Nothing replaces pr_url as a link,
-- because a draft's location is per-article rather than per-run.
ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS cms_namespace VARCHAR(255);

ALTER TABLE blog_runs DROP COLUMN IF EXISTS branch;
ALTER TABLE blog_runs DROP COLUMN IF EXISTS pr_number;
ALTER TABLE blog_runs DROP COLUMN IF EXISTS pr_url;

-- The converted body, stored next to the markdown it came from: the HTML is
-- what the CMS actually received, so keeping it makes a published post
-- reproducible without re-running the converter that produced it.
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS html TEXT NOT NULL DEFAULT '';

-- Where each article landed. post_uuid is opsapi's identifier, which is what a
-- user needs to open the draft in the CMS dashboard.
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS post_uuid   VARCHAR(255);
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS post_status VARCHAR(50);
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS posted_at   TIMESTAMPTZ;

-- Partial: only published articles carry a post_uuid, and the index exists to
-- answer "which article is this CMS draft?" rather than to scan the table.
CREATE INDEX IF NOT EXISTS idx_blog_articles_post_uuid
    ON blog_articles(post_uuid) WHERE post_uuid IS NOT NULL;

-- The generation trace gained a conversion step and lost the pull-request one.
-- Runs recorded before this migration keep a step key nothing renders, so
-- rewrite them: 'opening_pr' described committing to a repository, which is the
-- same phase 'publishing' now covers on its own.
UPDATE blog_runs
SET steps = (
    SELECT COALESCE(jsonb_agg(step ORDER BY ord), '[]'::jsonb)
    FROM jsonb_array_elements(steps) WITH ORDINALITY AS t(step, ord)
    WHERE step->>'key' <> 'opening_pr'
)
WHERE steps @> '[{"key": "opening_pr"}]'::jsonb;
