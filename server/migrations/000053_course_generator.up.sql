-- Migration 053: Course Generator agent seed + run and chapter tables.
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Course Generator',
    'Course Author',
    'Researches a subject and writes a structured course: outline, chapter theory, visuals and a quiz per chapter, saved for review before it goes to Workstation Academy.',
    'active',
    'go_native',
    'You are the Course Generator agent. Research the subject, plan a course of clear chapters with learning objectives, write accurate chapter theory for the stated audience and level, illustrate it, and write a multiple-choice quiz for each chapter. Cite only what research supports.',
    '{"builtin":"course_generator"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'course_generator'
);

CREATE TABLE IF NOT EXISTS course_runs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    task_id UUID REFERENCES tasks(id) ON DELETE SET NULL,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status VARCHAR(20) NOT NULL DEFAULT 'queued'
        CHECK (status IN ('queued', 'running', 'completed', 'failed', 'cancelled')),
    brief JSONB NOT NULL DEFAULT '{}'::jsonb,
    outline JSONB,
    steps JSONB NOT NULL DEFAULT '[]'::jsonb,
    cover_url TEXT NOT NULL DEFAULT '',
    sources JSONB NOT NULL DEFAULT '[]'::jsonb,
    warnings JSONB NOT NULL DEFAULT '[]'::jsonb,
    error_message TEXT,
    requested_by UUID,
    heartbeat_at TIMESTAMP,
    started_at TIMESTAMP,
    completed_at TIMESTAMP,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_course_runs_org_created ON course_runs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_course_runs_running ON course_runs(status) WHERE status = 'running';

-- One chapter = one Academy lesson. Locale is part of the key so a translated
-- course is more rows, not a schema change.
CREATE TABLE IF NOT EXISTS course_chapters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    run_id UUID NOT NULL REFERENCES course_runs(id) ON DELETE CASCADE,
    locale VARCHAR(16) NOT NULL DEFAULT 'en',
    position INT NOT NULL,
    title TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    objectives JSONB NOT NULL DEFAULT '[]'::jsonb,
    markdown TEXT NOT NULL DEFAULT '',
    html TEXT NOT NULL DEFAULT '',
    images JSONB NOT NULL DEFAULT '[]'::jsonb,
    quiz JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_course_chapters_run_locale_pos
    ON course_chapters(run_id, locale, position);
