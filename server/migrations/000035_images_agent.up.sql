-- Migration 035: seed the built-in Image Generator for existing orgs.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot. New organizations are seeded by auth_service.Register (imagesAgentSeed).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Image Generator',
    'Image',
    'Generates one image from a prompt and stores it on the Task Manager board.',
    'active',
    'go_native',
    'You generate images from a written prompt. Ask for the picture the user wants. Do not invent a subject.',
    '{"builtin":"images"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'images'
);
