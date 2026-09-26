-- Migration 046: remember how a blog run was asked to work, so Retry can
-- replay it.
--
-- A trending run is created with no topics and discovers them once it starts.
-- When discovery itself fails — the model gateway timing out, a reply that is
-- not JSON — the run is left with empty briefs, and Retry had nothing to replay:
-- it answered 400 "run has no topics to retry" while the UI kept offering the
-- button. Storing the trending flag, count, focus areas and auto-publish on the
-- run lets Retry run discovery again with the same steering.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot, so every statement here must be safe to run twice.

ALTER TABLE blog_runs ADD COLUMN IF NOT EXISTS options JSONB NOT NULL DEFAULT '{}';

-- Backfill scheduled runs that failed before choosing a topic, from the
-- schedule that started them. The scheduler records the blog run id in the
-- scheduled run's output, which is the only link between the two. Guarded on
-- empty options and empty briefs so the join only ever looks at the few rows
-- that still need it, and a replay after the backfill touches nothing.
UPDATE blog_runs br
SET options = jsonb_strip_nulls(jsonb_build_object(
        'trending',       true,
        'trending_count', st.input_json->'trending_count',
        'focus',          st.input_json->'focus',
        'auto_publish',   st.input_json->'auto_publish'))
FROM scheduled_task_runs str
JOIN scheduled_tasks st ON st.id = str.scheduled_task_id
WHERE br.options = '{}'::jsonb
  AND br.source = 'schedule'
  AND jsonb_array_length(br.briefs) = 0
  AND str.output LIKE '%' || br.id::text || '%'
  AND jsonb_typeof(st.input_json) = 'object'
  AND st.input_json->>'trending' = 'true';
