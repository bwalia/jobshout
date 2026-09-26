-- Migration 033: built-in Mail Agent, org Gmail connection, watched threads, drafts.
--
-- The Mail Agent watches one shared org Gmail, classifies inbound mail, drafts
-- replies (never auto-sends), and commissions the Research Agent when a reply
-- needs facts. A human approves before anything is sent.
--
-- IDEMPOTENCY IS MANDATORY. database/migrate.go replays every *.up.sql on every
-- boot — there is no schema_migrations table — so a statement that cannot run
-- twice takes down every environment on restart, not just this feature. Every
-- object below is created IF NOT EXISTS; the agent seed uses NOT EXISTS.
--
-- New organizations are seeded by auth_service.Register (mailAgentSeed); this
-- statement covers organizations that already existed when the feature shipped.
-- The two must stay in step with mailAgentSeed() in mail_service.go.

CREATE TABLE IF NOT EXISTS mail_connections (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    -- Connected Google account. Empty while disconnected.
    google_email TEXT NOT NULL DEFAULT '',
    -- AES-GCM ciphertext of the OAuth refresh token. Never stored plaintext.
    -- The encryption key is GMAIL_TOKEN_KEY (process env), not this row.
    refresh_token_enc BYTEA,
    token_expiry TIMESTAMPTZ,
    -- Scopes granted at connect time. Documented in mail.Scopes.
    scopes TEXT[] NOT NULL DEFAULT '{}',
    -- Labelling / archive after send. Defaults off; send-after-approve does
    -- not need this flag. Drafts live in JobShout, not as Gmail drafts.
    allow_mailbox_mutations BOOLEAN NOT NULL DEFAULT false,
    watch_labels TEXT[] NOT NULL DEFAULT '{}',
    watch_senders TEXT[] NOT NULL DEFAULT '{}',
    watch_subject_prefixes TEXT[] NOT NULL DEFAULT '{}',
    status VARCHAR(20) NOT NULL DEFAULT 'disconnected'
        CHECK (status IN ('disconnected', 'connected', 'error')),
    status_error TEXT,
    last_sync_at TIMESTAMPTZ,
    next_sync_at TIMESTAMPTZ,
    sync_lease_until TIMESTAMPTZ,
    connected_at TIMESTAMPTZ,
    disconnected_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mail_connections_next_sync
    ON mail_connections (next_sync_at)
    WHERE status = 'connected';

CREATE TABLE IF NOT EXISTS mail_oauth_states (
    state TEXT PRIMARY KEY,
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS mail_threads (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    connection_id UUID NOT NULL REFERENCES mail_connections(id) ON DELETE CASCADE,
    gmail_thread_id TEXT NOT NULL,
    gmail_message_id TEXT NOT NULL DEFAULT '',
    from_email TEXT NOT NULL DEFAULT '',
    from_name TEXT NOT NULL DEFAULT '',
    to_email TEXT NOT NULL DEFAULT '',
    subject TEXT NOT NULL DEFAULT '',
    snippet TEXT NOT NULL DEFAULT '',
    body_text TEXT NOT NULL DEFAULT '',
    message_id_header TEXT NOT NULL DEFAULT '',
    references_header TEXT NOT NULL DEFAULT '',
    received_at TIMESTAMPTZ,
    status VARCHAR(30) NOT NULL DEFAULT 'new'
        CHECK (status IN (
            'new', 'classifying', 'researching', 'draft_ready',
            'sent', 'rejected', 'ignored', 'failed'
        )),
    classification JSONB,
    needs_research BOOLEAN NOT NULL DEFAULT false,
    research_summary TEXT,
    research_findings JSONB,
    research_brief_id UUID,
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (org_id, gmail_thread_id)
);
CREATE INDEX IF NOT EXISTS idx_mail_threads_org_updated
    ON mail_threads (org_id, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_mail_threads_status
    ON mail_threads (org_id, status);

CREATE TABLE IF NOT EXISTS mail_drafts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    thread_id UUID NOT NULL UNIQUE REFERENCES mail_threads(id) ON DELETE CASCADE,
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    -- draft: editable, not sent. approved: human said yes; send may still be
    -- in flight / retryable. sent / rejected are terminal.
    status VARCHAR(20) NOT NULL DEFAULT 'draft'
        CHECK (status IN ('draft', 'approved', 'sent', 'rejected')),
    subject TEXT NOT NULL DEFAULT '',
    body TEXT NOT NULL DEFAULT '',
    to_email TEXT NOT NULL DEFAULT '',
    cc_email TEXT NOT NULL DEFAULT '',
    research_brief_id UUID,
    approved_by UUID REFERENCES users(id) ON DELETE SET NULL,
    approved_at TIMESTAMPTZ,
    rejected_by UUID REFERENCES users(id) ON DELETE SET NULL,
    rejected_at TIMESTAMPTZ,
    gmail_message_id TEXT,
    sent_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mail_drafts_org_status
    ON mail_drafts (org_id, status);

INSERT INTO agents (org_id, name, role, description, status, engine_type, system_prompt, metadata)
SELECT
    o.id,
    'Mail Agent',
    'Mail',
    'Watches the organisation Gmail inbox, drafts replies, and hands research to the Research Agent. Nothing is sent until a human approves.',
    'active',
    'go_native',
    'You are the Mail Agent. You triage the organisation inbox, draft replies, and never send until a human approves. You never claim a message was sent unless the send API succeeded after approval. Work that needs facts is handed to the Research Agent — you do not invent citations.',
    '{"builtin":"mail"}'::jsonb
FROM organizations o
WHERE NOT EXISTS (
    SELECT 1 FROM agents a
    WHERE a.org_id = o.id
      AND a.metadata->>'builtin' = 'mail'
);
