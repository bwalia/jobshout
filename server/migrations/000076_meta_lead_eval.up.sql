-- Migration 076: Meta Lead Analyst — human labels and model comparisons.
--
-- label_is_lead is a person's verdict on a DM, comment or mention (NULL: not
-- labelled). The labelled rows are the test set meta_lead_evals runs every
-- chosen model over, several times each, to measure accuracy and consistency
-- before META_LEAD_MODEL is changed.
--
-- IDEMPOTENCY IS MANDATORY (migrate.go replays every *.up.sql on boot).

ALTER TABLE meta_leads ADD COLUMN IF NOT EXISTS label_is_lead BOOLEAN;

CREATE INDEX IF NOT EXISTS idx_meta_leads_labelled
    ON meta_leads (org_id, updated_at DESC)
    WHERE label_is_lead IS NOT NULL;

CREATE TABLE IF NOT EXISTS meta_lead_evals (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    status        VARCHAR(32) NOT NULL DEFAULT 'running',
    models        JSONB       NOT NULL DEFAULT '[]',
    runs          INT         NOT NULL DEFAULT 3,
    case_count    INT         NOT NULL DEFAULT 0,
    report        JSONB,
    error_message TEXT        NOT NULL DEFAULT '',
    created_by    UUID,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_meta_lead_evals_org ON meta_lead_evals (org_id, created_at DESC);
