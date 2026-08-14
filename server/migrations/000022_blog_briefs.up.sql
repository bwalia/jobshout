-- Migration 022: give a run briefs instead of bare topics, and give an article
-- its own title and reference list.
--
-- Three changes:
--   1. blog_runs gains briefs — one {topic, context} object per article. A
--      topic string alone cannot carry the angle, audience or "avoid this"
--      guidance a writer needs, and a recurring schedule has nowhere to keep
--      standing instructions. topics is kept alongside it, still populated, so
--      existing rows and anything reading it keep working.
--   2. blog_articles gains title — until now the title was re-derived from the
--      markdown's H1 on every read, which meant the agent could not choose a
--      title from what it learned while researching.
--   3. blog_articles gains references_json — the verified sources the article
--      was written from, stored as structured JSON rather than prose at the
--      bottom of the markdown, so they can be rendered, checked and re-used.
--      The column is not called "references" because REFERENCES is a reserved
--      word in SQL; naming it so would force every query touching it to quote
--      the identifier forever.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart, not just this feature.

ALTER TABLE blog_runs     ADD COLUMN IF NOT EXISTS briefs     JSONB NOT NULL DEFAULT '[]';
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS title      TEXT;
ALTER TABLE blog_articles ADD COLUMN IF NOT EXISTS references_json JSONB NOT NULL DEFAULT '[]';

-- Backfill briefs from topics for runs that predate this column, so an old run
-- can still be retried, read and rendered through the new code path. Guarded on
-- emptiness rather than on NULL: the column has a '[]' default, so every
-- existing row already has a value that is merely not populated yet.
--
-- jsonb_array_elements_text over topics preserves order, and the context is
-- empty because these runs never had one to record.
UPDATE blog_runs
SET briefs = (
    SELECT COALESCE(jsonb_agg(jsonb_build_object('topic', t, 'context', '')), '[]'::jsonb)
    FROM jsonb_array_elements_text(topics) AS t
)
WHERE jsonb_array_length(briefs) = 0
  AND jsonb_array_length(topics) > 0;

-- Backfill titles from the markdown H1, matching what the old read path derived
-- at render time, so existing articles do not display an empty title.
--
-- The pattern takes the first line beginning with "# " and stops at the newline.
-- Articles whose markdown has no H1 keep a NULL title and fall back to the
-- topic, exactly as before.
UPDATE blog_articles
SET title = NULLIF(TRIM(SUBSTRING(markdown FROM '(?n)^#[[:space:]]+(.*)$')), '')
WHERE title IS NULL;

-- Phase 2's trending discovery needs to know what has been written recently so
-- it does not propose the same subject twice in a week. This index makes that
-- lookup cheap; it is created here, with the schema it depends on, rather than
-- left for the feature that will use it.
CREATE INDEX IF NOT EXISTS idx_blog_articles_org_created
    ON blog_articles(org_id, created_at DESC);
