-- Migration 067: org routing mode.
--
-- replace  — every call goes down the org chain (the behaviour of 065).
-- fallback — each agent's own provider and model answer first; the chain only
--            takes over when that call fails (a busy or unreachable self-hosted
--            model, a key over quota).
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).
ALTER TABLE org_llm_routing ADD COLUMN IF NOT EXISTS mode VARCHAR(20) NOT NULL DEFAULT 'replace';
