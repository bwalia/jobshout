# AB Testing Agent

Builtin `ab_testing` — manages **wslproxy** weighted / canary traffic splits and
observes live assignment. This is JobShout’s control plane for experiments that
used to be demoed only at https://abtesting.fictionally.org/.

## Pieces

| Component | Role |
| --- | --- |
| `server/internal/abtest` | Module, demo fixtures, observe client |
| `server/internal/wslproxymcp` | Typed MCP tools (`update_traffic_split`, …) |
| `server/internal/mcp` | Generic JSON-RPC MCP client (also usable for other MCP servers) |
| `GET|POST /api/v1/ab-testing/*` | HTTP façade |
| `AbTestAgentClient` | Task Manager tab |

## Actions

- `list` — experiments (demo or live)
- `set_weights` — MCP `update_traffic_split`
- `promote` / `rollback` — MCP promote/rollback
- `observe` — N GETs to `https://{host}{observe_path}` (default `/version`)

Without MCP credentials the agent stays in **demo mode** (fixtures + synthetic observe).

See [wslproxy-mcp.md](./wslproxy-mcp.md) for MCP setup inside and outside JobShout.
