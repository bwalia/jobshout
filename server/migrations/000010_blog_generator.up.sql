-- Auto blog generator: persist each run so operators can list generated PRs.

CREATE TABLE IF NOT EXISTS blog_runs (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id        UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    triggered_by  UUID REFERENCES users(id) ON DELETE SET NULL,
    -- Source of this run: api | schedule — lets us filter cron runs.
    source        VARCHAR(50) NOT NULL DEFAULT 'api',
    status        VARCHAR(50) NOT NULL DEFAULT 'pending',
    topics        JSONB NOT NULL DEFAULT '[]',
    model         VARCHAR(100),
    branch        VARCHAR(255),
    pr_number     INTEGER,
    pr_url        VARCHAR(500),
    articles      JSONB NOT NULL DEFAULT '[]',
    error_message TEXT,
    started_at    TIMESTAMPTZ,
    completed_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_blog_runs_org_created ON blog_runs(org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_blog_runs_status ON blog_runs(status);
