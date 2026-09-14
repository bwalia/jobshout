# wslproxy MCP — inside and outside JobShout

JobShout talks to **wslproxy** two ways:

| Path | Used by | Auth |
| --- | --- | --- |
| REST `/api/*` | WAF Efficacy Lab (`waflab.Client`); AB Testing rule reads, and its writes when MCP tools are off | Bearer API token or login |
| MCP `/mcp/jsonrpc` | AB Testing Agent writes when tools are on (`wslproxymcp` → `internal/mcp`) | `X-MCP-API-Key` |

Prefer MCP for traffic engineering (`update_traffic_split`, `promote_backend`,
`rollback_backend`, `bind_waf_policy`, server/rule CRUD). Prefer REST for the
WAF attack relay (`POST /api/waf/test`) and bulk `projects/import`.

## Troubleshooting

### `tools/list` returns `{"tools":{}}` / error `-32002 MCP tools are disabled`

wslproxy answers `initialize` but advertises no tools while `mcp.tools_enabled`
is false or `mcp.mode` is not `read-write` (its Lua encoder writes the empty
list as `{}`, which JobShout reads as zero tools). The AB Testing tab then
says so in its status line and writes over the admin API instead, if
`WSLPROXY_USERNAME` + `WSLPROXY_PASSWORD` (or `WSLPROXY_API_TOKEN`) are set.
To use MCP, set `mcp.tools_enabled=true`, `mcp.mode="read-write"` and
`mcp.api_key` in wslproxy `settings.json`, and the same key as
`WSLPROXY_MCP_API_KEY` in JobShout.

### `unexpected status 405` / redirect to `/login`

`https://lon1.pop0.uk/mcp/*` must hit OpenResty’s MCP location. If nginx’s
Next.js admin `server` block is missing `/mcp`, requests fall through to the
dashboard UI, which `307`s to `/login`. Following that redirect with POST
yields **405 Method Not Allowed**.

Fix: deploy wslproxy with the `/mcp` location on the Next.js admin port
(see wslproxy PR for `nginx.conf.j2`). JobShout’s MCP client refuses redirects
so the error names the Location instead of looking like a bogus 405.

Smoke without following redirects:

```bash
curl --max-redirs 0 -X POST "$WSLPROXY_BASE_URL/mcp/jsonrpc" \
  -H "Content-Type: application/json" \
  -H "X-MCP-API-Key: $WSLPROXY_MCP_API_KEY" \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}'
```

## Inside JobShout

1. Set `WSLPROXY_BASE_URL` (admin POP, e.g. `https://lon1.pop0.uk`).
2. Set `WSLPROXY_MCP_API_KEY` to the same value as wslproxy `settings.json` →
   `mcp.api_key` (header defaults to `X-MCP-API-Key`).
3. Enable MCP write mode on wslproxy (`mcp.mode="read-write"`, `mcp.tools_enabled=true`).
   Without it the AB Testing agent falls back to the admin API for writes.
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

See [ab-testing-agent.md](./ab-testing-agent.md). In short: the agent reads the
host's real wslproxy rule and only writes when that rule is the host's own
split. abtesting.fictionally.org currently shares a single-backend rule with
other hosts, and its 80/20 split lives in the k3s1 Traefik IngressRoute
`default/webapp-ingress`, so the tab is read-only for it (Observe still works).

Env: `WSLPROXY_AB_DEMO_HOST`, `WSLPROXY_AB_RULE_ID`, `WSLPROXY_AB_OBSERVE_PATH`.
