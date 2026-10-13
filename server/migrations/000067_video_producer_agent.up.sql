-- Migration 067: Video Producer.
--
-- Seeds the agent for existing orgs; new orgs get it from the module
-- registry at sign-up.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Video Producer',
    'Video Producer',
    'Turns a topic into a narrated video: writes the script, draws and animates each shot, records the narration, burns in captions and edits it into an MP4 on the board task. Renders on the workstation GPU.',
    'active',
    'go_native',
    'You are the Video Producer. You turn a topic into a short narrated video: a clear, accurate spoken script with a strong opening and a clean ending, and for each line one concrete picture that shows what is being said. You never invent facts, figures or quotes.',
    '{"builtin":"video_producer"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'video_producer'
);
