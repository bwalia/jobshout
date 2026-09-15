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

## abtesting.fictionally.org

The host has its own weighted wslproxy rule, so the tab is writable for it.

```
browser ─► lon1.pop0.uk (wslproxy)
             host:abtesting.fictionally.org
               rule abtesting-split (5f281375-14fe-62c2-9b45-ca6689d74ece, weighted)
                 v1 ─► http://origin-uk-001.pop0.uk:30086 ─► default/webapp-v1-edge (cloud001)
                 v2 ─► http://origin-uk-001.pop0.uk:30087 ─► default/webapp-v2-edge (cloud001)
```

| File | What |
| --- | --- |
| [`deploy/abtesting/webapp-edge.yaml`](../deploy/abtesting/webapp-edge.yaml) | v1/v2 Deployments + NodePort Services on k3s1 |
| [`deploy/abtesting/wslproxy-rule-abtesting-split.json`](../deploy/abtesting/wslproxy-rule-abtesting-split.json) | The wslproxy rule (weights as first created: 80/20) |

The live weights are whatever the tab last applied; the JSON is the starting
point, not a source of truth. `GET /api/v1/ab-testing/experiments/abtesting`
(or the tab) shows the current values.

### Why it is built this way

- **Own rule, not the shared one.** Before this, the host used
  `aws-k3s1-ingress-13.42.188.136` (`8f161403-…`), which also serves
  int.jobshout.co.uk, jenkins, ring-promoter, shop, int-langfuse and others.
  Changing weights there would move all of them.
- **Pinned to cloud001.** The edge cannot route to pods on the LAN nodes, so a
  NodePort only answers from outside when its pod runs on cloud001. The pods
  tolerate cloud001's `node-role.kubernetes.io/edge=true:NoSchedule` taint.
- **Traefik is out of the path for this host.** The older IngressRoute
  `default/webapp-ingress` (80/20 across `webapp-v1`/`webapp-v2`) and those two
  Deployments still exist but no longer carry abtesting.fictionally.org
  traffic, so the splits do not multiply.

### Rebuild from scratch

1. `kubectl --kubeconfig ~/.kube/k3s1.yaml apply -f deploy/abtesting/webapp-edge.yaml`,
   then check `curl http://origin-uk-001.pop0.uk:30086/version` → `v1` and
   `:30087` → `v2`.
2. `POST https://lon1.pop0.uk/api/rules` with the rule JSON (without `servers`).
   wslproxy generates the id; set `WSLPROXY_AB_RULE_ID` to it if you want to pin it.
3. `GET /api/servers/host%3Aabtesting.fictionally.org`, set `rules` to
   `["<new id>"]`, and `PUT` it back — with `config` **base64-decoded** (see below).
4. `GET /api/rules/<new id>`; if `servers` does not list
   `host:abtesting.fictionally.org`, `PUT` the rule back with it added.
5. Verify: `curl -sD- https://abtesting.fictionally.org/version` shows
   `x-wsl-rule: abtesting-split`, and a hundred requests land roughly 80/20.

### wslproxy admin API gotchas

- **`config` round-trip.** `GET /api/servers/{id}` returns `config` base64, but
  `PUT` expects the plain nginx text and encodes it again. PUT what GET returned
  and the stored config becomes base64-of-base64; traffic keeps working until
  the next nginx reload writes that garbage to the host's `.conf`. Always
  decode `config` before a PUT, then confirm GET returns the original value.
- **Re-saving a server's current rule unlinks it.** A `PUT` of a server whose
  `rules` already names a rule removes the host from that rule's `servers` and
  does not add it back. Routing follows the server's `rules`, but the admin UI
  and JobShout's shared-rule check read the rule's `servers`, so re-add it
  (step 4).

### Roll back to the shared rule

`PUT /api/servers/host%3Aabtesting.fictionally.org` with `rules` set to
`["8f161403-8592-1111-6294-9c57974505b0"]` (config decoded). wslproxy moves the
host back onto the shared rule's `servers`; the tab then shows the host
read-only again. The `abtesting-split` rule and the `*-edge` Kubernetes objects
can be deleted afterwards.

See [wslproxy-mcp.md](./wslproxy-mcp.md) for MCP setup inside and outside JobShout.
