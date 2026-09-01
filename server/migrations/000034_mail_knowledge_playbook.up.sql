-- Migration 034: Mail Agent knowledge playbook on the org mailbox.
--
-- Operators pin http(s) URLs plus a research question and reply style. Matching
-- mail is researched from those pages only, then drafted in that style.
-- Empty watch_knowledge_urls keeps the existing needs_research + open-web path.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table.

ALTER TABLE mail_connections
    ADD COLUMN IF NOT EXISTS watch_knowledge_urls TEXT[] NOT NULL DEFAULT '{}';

ALTER TABLE mail_connections
    ADD COLUMN IF NOT EXISTS research_focus TEXT NOT NULL DEFAULT '';

ALTER TABLE mail_connections
    ADD COLUMN IF NOT EXISTS reply_instructions TEXT NOT NULL DEFAULT '';
