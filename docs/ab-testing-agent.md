# AB Testing Agent

Builtin `ab_testing` — manages **wslproxy** weighted / canary traffic splits and
observes live assignment for one public host (`WSLPROXY_AB_DEMO_HOST`, default
`abtesting.fictionally.org`).

## Pieces

| Component | Role |
| --- | --- |
| `server/internal/abtest` | Module, experiment resolution, write-path choice, observe, demo fixtures |
| `server/internal/waflab` (`traffic.go`) | wslproxy admin API: server/rule reads, `POST /api/traffic/backends/*` |
| `server/internal/wslproxymcp` | Typed MCP tools (`update_traffic_split`, …) and the tools probe |
| `server/internal/mcp` | Generic JSON-RPC MCP client |
| `GET|POST /api/v1/ab-testing/*` | HTTP façade |
| `AbTestAgentClient` | Task Manager tab |

## What it reads

Nothing about the experiment is invented. On every list/status the agent reads
from the wslproxy admin API (same credentials as the WAF lab):

1. `GET /api/servers/host:<host>` — the rules attached to the host.
2. `GET /api/rules/<id>` — `WSLPROXY_AB_RULE_ID` if set, otherwise the host's
   rule (preferring one with two or more backends).
3. The rule's `match.response.backends` (label, address, weight) and `servers`.

## When it will write

An experiment is **writable** only when all of these hold; otherwise the tab
shows it read-only with the reason, and the API refuses writes with **409**:

- the rule is attached to the host, and to **no other host** — a shared rule is
  never changed, because its weights move every host on it;
- the rule has **two or more backends**;
- a write path exists:
  - **MCP** when wslproxy advertises `update_traffic_split` etc., else
  - **admin API** (`POST /api/traffic/backends/{weights,promote,rollback}`)
    when `WSLPROXY_USERNAME` + `WSLPROXY_PASSWORD` or `WSLPROXY_API_TOKEN` are set.
    A tool call that fails with `-32002` (tools disabled) also falls back here.

Unknown backend labels or weights outside 0–100 get **422**. Failures from
wslproxy itself are **502** with its message.

## Status modes

| `mode` | Meaning |
| --- | --- |
| `demo` | No wslproxy configured — fixtures and synthetic observe |
| `live` | A write path works (`write_path`: `mcp` or `rest`); each experiment still carries its own `writable` |
| `read_only` | MCP reachable but no tools, and no admin credentials |
| `unavailable` | wslproxy configured but not answering |

## Actions

- `list` — the resolved experiment
- `set_weights` — `stable_weight` / `canary_weight` go to the rule's first and second backends
- `promote` — `promote_label`, or the second backend when empty
- `rollback` — back to single-backend routing
- `observe` — N GETs to `https://{host}{observe_path}` (default `/version`); works even when read-only

## abtesting.fictionally.org today

`host:abtesting.fictionally.org` uses rule `aws-k3s1-ingress-13.42.188.136`
(`8f161403-…`), which is shared with seven other hosts and has one backend (the
k3s1 ingress at 100%). The visible 80/20 split is done behind wslproxy by the
Traefik IngressRoute `default/webapp-ingress` (`webapp-v1` weight 80,
`webapp-v2` weight 20). So the tab is read-only for this host and Observe shows
the real split.

To manage it from JobShout, give the host its own wslproxy rule with one backend
per variant (for example the `webapp-v1` and `webapp-v2` services exposed to
wslproxy), attach only that rule to `host:abtesting.fictionally.org`, and
optionally pin it with `WSLPROXY_AB_RULE_ID`. Keep the Traefik weights at a
single variant, or the two splits multiply.

See [wslproxy-mcp.md](./wslproxy-mcp.md) for MCP setup inside and outside JobShout.
