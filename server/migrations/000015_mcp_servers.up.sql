-- Migration 015: MCP (Model Context Protocol) servers

-- One row per MCP server an org configures. Agents connect to the org's
-- enabled servers at execution time to discover and invoke their tools.
CREATE TABLE IF NOT EXISTS mcp_servers (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    org_id       UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name         TEXT NOT NULL,
    transport    TEXT NOT NULL DEFAULT 'http' CHECK (transport IN ('http')),
    url          TEXT NOT NULL,
    auth_header  TEXT,
    enabled      BOOLEAN NOT NULL DEFAULT true,
    created_by   UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_mcp_servers_org ON mcp_servers(org_id);
