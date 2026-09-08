# WAF Efficacy Lab

JobShout can provision a before/after WAF pair on **wslproxy** and measure an
honest attack matrix: one host with the WAF on (`secure`), one with it off
(`open`), both in front of the same origin.

The lab does not invent a WAF. It drives wslproxy’s existing rules, policies,
vhosts, optional Cloudflare DNS, and the server-side `POST /api/waf/test` relay.

## Pieces

| Component | Role |
| --- | --- |
| `server/internal/waflab` | Client, catalogue, phases, scoring, agent module |
| `server/internal/service/waflab_service.go` | Run lifecycle (queued → running → completed/failed) |
| `server/internal/handler/waflab_handler.go` | `/api/v1/waf-lab/*` |
| `web/nextjs/components/WafLabAgentClient.tsx` | Task Manager tab UI + matrix |

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `WSLPROXY_ENABLED` | `true` | Feature flag |
| `WSLPROXY_BASE_URL` | *(unset)* | e.g. `https://lon1.pop0.uk`. Unset disables the agent |
| `WSLPROXY_API_TOKEN` | *(unset)* | Bearer token (preferred) |
| `WSLPROXY_USERNAME` / `WSLPROXY_PASSWORD` | *(unset)* | Login when no token |
| `WSLPROXY_PLATFORM` | `openresty-admin-next` | `x-platform` header for mutation gate |
| `WSLPROXY_PROFILE` | `prod` | wslproxy env profile |
| `WSLPROXY_TIMEOUT` | `30s` | One HTTP call |
| `WSLPROXY_LAB_MAX_RUNTIME` | `10m` | Whole-run backstop |
| `WSLPROXY_TARGET_ALLOWLIST` | *(empty)* | Extra host allow-list on the JobShout side |
| `CLOUDFLARE_API_TOKEN` | *(unset)* | Optional; absent → DNS phase skipped |
| `CLOUDFLARE_ZONE` | *(unset)* | Optional zone pin |

Secrets are never logged, returned by status/Ready, or stored on run rows.

## Honest limits

- **BOLA/IDOR** (`GET /api/accounts/9999`) is not blocked by design. Object-level
  authorization has no signature in the request; a signature WAF cannot claim
  it. The score reports this as `not_blocked_expected`.
- **Scanner User-Agent detection** cannot be tested from a browser (scripts may
  not set `User-Agent`), but **can** through the `POST /api/waf/test` relay —
  that is why the catalogue includes it.
- A **100% block rate with zero false positives** on a non-trivial set is
  treated as suspicious; inspect raw responses rather than celebrating it.
- The relay only fires at hosts on wslproxy’s `waf.test_targets` allow-list.
  Hosts outside that list fail clearly; do not work around the guard.

## Modes

- `provision_and_test` — seed rules/policy, upsert hosts, optional DNS, then matrix
- `provision_only` — stop after hosts/DNS
- `test_only` — skip provision; fire the matrix at existing hosts

## API

- `GET /api/v1/waf-lab/status`
- `GET|POST /api/v1/waf-lab/runs`
- `GET /api/v1/waf-lab/runs/{id}`
- `GET /api/v1/waf-lab/runs/{id}/steps`
- `GET /api/v1/waf-lab/runs/{id}/results`
- `POST /api/v1/waf-lab/runs/{id}/cancel`
