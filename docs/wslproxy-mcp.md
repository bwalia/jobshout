# wslproxy MCP — inside and outside JobShout

JobShout talks to **wslproxy** two ways:

| Path | Used by | Auth |
| --- | --- | --- |
| REST `/api/*` | WAF Efficacy Lab (`waflab.Client`) | Bearer API token or login |
| MCP `/mcp/jsonrpc` | AB Testing Agent (`wslproxymcp` → `internal/mcp`) | `X-MCP-API-Key` |

Prefer MCP for traffic engineering (`update_traffic_split`, `promote_backend`,
`rollback_backend`, `bind_waf_policy`, server/rule CRUD). Prefer REST for the
WAF attack relay (`POST /api/waf/test`) and bulk `projects/import`.

## Inside JobShout

1. Set `WSLPROXY_BASE_URL` (admin POP, e.g. `https://lon1.pop0.uk`).
2. Set `WSLPROXY_MCP_API_KEY` to the same value as wslproxy `settings.json` →
   `mcp.api_key` (header defaults to `X-MCP-API-Key`).
3. Enable MCP write mode on wslproxy (`mcp.mode=write`, `mcp.tools_enabled=true`).
4. Open the **AB Testing** Task Manager tab (or `agent_execute` with builtin
   `ab_testing`).

Go packages:

- [`server/internal/mcp`](../server/internal/mcp) — generic Streamable HTTP MCP client
- [`server/internal/wslproxymcp`](../server/internal/wslproxymcp) — typed wslproxy tools
- [`server/internal/abtest`](../server/internal/abtest) — AB Testing agent module

You can also register the same MCP URL as an **org MCP server** in JobShout
(Integrations → MCP) so chat can `tools/list` / `tools/call` it through the
existing MCP catalogue — same client, different UI.

## Outside JobShout (Cursor / Claude Desktop)

Point any MCP client at the admin POP:

```json
{
  "mcpServers": {
    "wslproxy": {
      "url": "https://lon1.pop0.uk/mcp/jsonrpc",
      "headers": {
        "X-MCP-API-Key": "<mcp.api_key from wslproxy settings.json>"
      }
    }
  }
}
```

Or use the REST tool endpoints (`GET /mcp/tools`, `POST /mcp/tools/{name}`) —
see wslproxy `api/mcp/README.md`.

## AB Testing agent vs abtesting.fictionally.org

The public host remains the **edge experiment**. JobShout is the **control
plane**: set weights, promote/rollback, and observe `/version` samples with a
dot plot (expected vs observed). That replaces the standalone demo UI at
https://abtesting.fictionally.org/.

Env: `WSLPROXY_AB_DEMO_HOST`, `WSLPROXY_AB_RULE_ID`, `WSLPROXY_AB_OBSERVE_PATH`.
