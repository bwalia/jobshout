-- Migration 036: operator-written knowledge notes on the org mailbox.
--
-- The operator types what the Mail Agent should know (prices, products,
-- policies) straight into a text box; the drafter uses it verbatim instead of
-- researching pinned pages. Empty knowledge_notes keeps the existing
-- research-driven path.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table.

ALTER TABLE mail_connections
    ADD COLUMN IF NOT EXISTS knowledge_notes TEXT NOT NULL DEFAULT '';
