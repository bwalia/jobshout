# Prompt — build the "WAF Efficacy Lab" agent in JobShout

Paste everything below the line into a Claude Code session started in
`/Users/balinderwalia/projects/jobshout`.

---

# Task: add a built-in JobShout specialist — **WAF Efficacy Lab**

You are working in the JobShout repo at `/Users/balinderwalia/projects/jobshout`
(git remote already set up). A second repo, **wslproxy**, lives at
`/Users/balinderwalia/Documents/Work/wslproxy`. Read from it, but **do not change
it** unless a step below says so explicitly, and then only as a separate branch and PR
in that repo.

Build a new built-in agent, **WAF Efficacy Lab**, that stands up and measures a
before/after WAF test lab on wslproxy: one host with the WAF on, one host with the WAF
off, both in front of the same deliberately vulnerable origin. The agent talks to
wslproxy over its HTTP API with an API token, pushes the WAF rules and policy, binds
them to the vhosts, optionally creates the DNS records through Cloudflare, then fires
the attack matrix and reports how many attacks each host stopped.

The finished agent runs on **https://int.jobshout.co.uk** (the `int` ring).

---

## 1. Read these first — do not invent what already exists

Most of the lab is already built in wslproxy. Your job is to drive it from JobShout,
not to rebuild it.

**In wslproxy — the lab and the WAF:**

| Path | What it gives you |
| --- | --- |
| `examples/wslproxy-waf-demo/README.md` | The whole lab, end to end. Read this before anything else. |
| `examples/wslproxy-waf-demo/data/waf_rules/prod/*.json` | The 17 modern rule JSONs (SSTI, Log4Shell, SSRF, NoSQLi, XXE, JWT-none, prototype pollution, GraphQL, smuggling, …). |
| `examples/wslproxy-waf-demo/data/waf_policies/prod/waf-policy-payments-hard.json` | Block-mode policy, 37 rules, anomaly threshold 6, plus v2 stage config. |
| `examples/wslproxy-waf-demo/data/servers/prod/host:payments-*.json` | The two vhosts. Identical except `waf_enabled`, `waf_policy_id`, `waf_mode_override`. |
| `examples/wslproxy-waf-demo/data/rules/prod/payments-demo-default.json` | Routing rule: 305 proxy to `127.0.0.1:30084`. |
| `examples/wslproxy-waf-demo/test_waf_live.py` | The full live matrix — every signature and every v2 stage, asserts secure blocks and open does not. **This is the source of truth for the attack catalogue.** |
| `examples/wslproxy-waf-demo/waf_features.py` | 16 golden tests for the v2 enforcement stages. |
| `examples/wslproxy-waf-demo/attack_suite.py` | The 20-attack before/after matrix, `--json` for machine output. |
| `examples/wslproxy-waf-demo/k3s1-payments-api.yaml` | The vulnerable origin (ConfigMap-carried Python app, NodePort 30084, pinned to `cloud001`). |
| `api/api.lua` | The API. See §2 below for the exact endpoints. |
| `api/waf_default_rules.lua` | The 20 base OWASP rules and the `POST /api/waf_rules/seed` seed data. |
| `api/dns_manager.lua` | Cloudflare provisioning (`find_zone`, `ensure_record`, `provision_for_server`). |
| `docs/WAF_ENGINE_V2.md`, `docs/waf-policy.schema.json` | Policy shape and the v2 engine design. |

**In JobShout — how a specialist is wired:**

| Path | Why |
| --- | --- |
| `.claude/rules/agent-modules.md` | **The binding rule.** Read it and obey it. |
| `server/internal/agentmodule/registry.go` | The `Module` struct you must fill in. |
| `server/internal/agentmodules/register.go` | The single `Register` call and the `Deps` struct. |
| `server/internal/pentester/module.go` | Closest template — a security agent with an external runner, requirements and a `Ready` check. Copy its shape. |
| `server/internal/simpro/{client.go,module.go}` | Closest template for an **external HTTP API with a token**, env-loaded config, and a demo/live split. Copy its shape for the wslproxy client. |
| `server/internal/service/pentest_service.go` | Run records, status, persistence. |
| `server/migrations/000042_simpro_payments_agent.up.sql` | The seed-migration pattern. Idempotent, replayed on every boot. |
| `web/nextjs/lib/agents/tab-clients.tsx` | Where a tab UI registers. |
| `web/nextjs/components/PentestAgentClient.tsx` (+ `PentestRunForm/Result/List`) | The tab UI shape to follow. |
| `docs/pentest-agent.md` | The doc style for an agent that talks to an outside service. |

---

## 2. The wslproxy API contract

Base URL comes from config; the `int` ring points at **`https://lon1.pop0.uk`**.

**Auth.** `POST /api/user/login` with valid credentials returns an access token. Every
later call sends `Authorization: Bearer <token>`. Cache the token and refresh on 401 —
JWT expiry is 3600s (`Helper.generateToken`).

**Endpoints you need:**

| Method + path | Use |
| --- | --- |
| `POST /api/user/login` | Get the bearer token. |
| `GET/POST /api/waf_rules`, `GET/PUT/DELETE /api/waf_rules/{id}` | Rule CRUD. |
| `POST /api/waf_rules/seed` | Seed the 20 base OWASP rules + base policy. |
| `GET/POST /api/waf_policies`, `/api/waf_policies/{id}` | Policy CRUD. |
| `GET /api/waf_events` | Recent block events (from the `waf_events` shared dict). |
| `POST /api/waf/test` | **Server-side single-request relay.** Body `{target, method, path, headers, body, content_type, timeout_ms}`. Returns `{ok, status, blocked, waf_block, waf_rule, waf_violation, support_id, latency_ms, body_snippet}`. |
| `GET /api/waf/test/targets` | The SSRF allow-list this relay will fire at. |
| `GET/POST /api/servers`, `/api/servers/{id}` | The two vhosts (`waf_enabled`, `waf_policy_id`, `waf_mode_override`). |
| `GET/POST /api/rules` | The routing rule to the origin. |
| `POST /api/projects/import` | Bulk import: body `{dataType, data[]}` where `dataType` is `waf_rules`, `waf_policies`, `rules` or `servers`. |
| `POST /api/dns/provision` | Cloudflare provisioning for a server record (`server_id`, `profile_id`, flags). |
| `GET /api/dns/lookup` | Check what DNS already exists. |

**Three gotchas that will cost you a day if you miss them:**

1. **`POST /api/projects/import` form-parses its body.** It runs
   `ngx.req.get_post_args` → `Helper.GetPayloads`, so any `&` or `=` inside the JSON
   (the `&&` cmdi regex, the base64 block page) truncates the body and you get a 500.
   **Percent-encode the whole JSON body before posting**; the gateway decodes it back.
   Put this in the client, not in the caller, and cover it with a test.

2. **The mutation gate.** `api/api.lua` refuses writes unless
   `settings.instance_locked == "false"` or the request carries an `x-platform` header
   on the allow-list — today only `react-admin`, `openresty-admin-next`,
   `openresty-admin-next-ssr`. JobShout is none of these. Make the header value a
   config setting (`WSLPROXY_PLATFORM`, default `openresty-admin-next`) so the agent
   works today, and **open a separate small PR on wslproxy** adding a `jobshout-agent`
   entry to `ALLOWED_MUTATION_PLATFORMS` with a comment saying why. Do not silently
   impersonate the admin UI without leaving that note in the code.

3. **`POST /api/waf/test` only fires at allow-listed hosts** (`settings.waf.test_targets`,
   defaulting to the three `payments-*.fictionally.org` hosts), never at a private,
   loopback, link-local, CGNAT or metadata address. If the agent is asked to test a host
   that is not on the list, fail with a clear message naming the allow-list — do not try
   to work around the guard. Verify the target against `GET /api/waf/test/targets` before
   firing anything.

**Do not** reach around the API to the wslproxy filesystem, kubectl, or SSH. The API is
the whole interface.

---

## 3. What the agent actually does

One launch, five phases. Each phase is skippable by a schema field, and each writes a
step record so the run reads as a story in the UI.

1. **Preflight.** Log in, read `GET /api/waf/test/targets`, list existing
   `waf_rules`/`waf_policies`/`servers`. Report what is already there. If everything is
   already in place, say so and go straight to phase 4.

2. **Rules and policy.** `POST /api/waf_rules/seed` for the 20 base rules, then import
   the 17 modern rules and `waf-policy-payments-hard` from the wslproxy demo payloads.
   Idempotent: match on rule `id`, update rather than duplicate.

3. **Hosts.** Create or update the routing rule and the two vhosts — the secure one with
   `waf_enabled: true`, `waf_policy_id: waf-policy-payments-hard`,
   `waf_mode_override: block`; the open one with `waf_enabled: false`. Both point at the
   same origin. Any drift from that pairing invalidates the whole before/after claim, so
   assert it and fail loudly if the two hosts differ in anything else.

4. **DNS (only when a Cloudflare token is present).** Create the CNAMEs
   `payments-{open,secure}.<zone> → lon1.pop0.uk`, **unproxied** (grey cloud) so auto-ssl
   can solve the ACME HTTP-01 challenge. Prefer `POST /api/dns/provision` over calling
   Cloudflare directly, so wslproxy's `managed_zones` allow-list stays the safety
   boundary. With no token, skip this phase and tell the user which records to create by
   hand. Then poll HTTPS on both hosts until the certs issue (bounded, ~3 min) and check
   the wslproxy default endpoint is alive at `https://lon1.pop0.uk/login`.

5. **Efficacy run.** Fire the attack catalogue at both hosts through
   `POST /api/waf/test` and build the matrix: per attack — category, payload, method,
   status, `blocked`, `x-waf-rule`, `x-waf-violation`, `x-support-id`, latency. Score it:
   attacks blocked on secure, attacks that got through on open, false positives (benign
   requests that were blocked — these matter more than the block count), and per-category
   coverage. Cross-check against `GET /api/waf_events`.

**Report honestly.** The lab's whole value is that it does not oversell. Two facts the
report must always carry:

- **BOLA/IDOR (`GET /api/accounts/9999`) is not blocked, by design.** Object-level
  authorization has no signature in the request; no signature WAF blocks it. Say so.
- **Scanner detection cannot be tested from a browser** (scripts may not set
  `User-Agent`) — but it *can* through the `POST /api/waf/test` relay, which is exactly
  why the relay exists. Include it.

A "100% blocked" score is a bug in the test, not a win. If false positives are zero and
every attack blocked, say the matrix looks suspicious and show the raw responses.

---

## 4. Files to create

Follow `.claude/rules/agent-modules.md`: **own package, one `Register` call, no
switches anywhere in the platform.**

```
server/internal/waflab/
  client.go        wslproxy HTTP client — login/token cache, percent-encoded import,
                   x-platform header, typed calls for the endpoints in §2.
  client_test.go   httptest server. Cover: token refresh on 401, the percent-encoding
                   of a body containing '&' and '=', a non-allow-listed target refused.
  lab.go           The five phases. Pure-ish functions over the client so they test
                   without a network.
  catalogue.go     The attack catalogue, generated from / kept in step with
                   examples/wslproxy-waf-demo/test_waf_live.py. Add a test that fails
                   when the two drift.
  score.go         Matrix → score. Blocked, leaked, false positives, per-category.
  module.go        agentmodule.Module — Builtin, Label, Icon, TabSlug, Hint, ChatHint,
                   Schema, Seed, Launch, Requirements, Ready.
  module_test.go

server/internal/service/waflab_service.go        Run lifecycle + persistence.
server/internal/service/waflab_service_test.go
server/internal/handler/waflab_handler.go        /api/v1/waf-lab/*
server/internal/repository/…                     runs repo, alongside the pentest one.
server/internal/model/waf_lab.go                 BuiltinWAFLab = "waf_lab", request/
                                                 result types, run + step + finding.
server/migrations/000043_waf_lab_agent.up.sql    Seed the builtin for existing orgs.
server/migrations/000043_waf_lab_agent.down.sql
server/migrations/000044_waf_lab_runs.up.sql     Runs, steps, per-attack results.
server/migrations/000044_waf_lab_runs.down.sql

web/nextjs/components/WafLabAgentClient.tsx      Tab UI.
web/nextjs/components/waflab/WafLabRunForm.tsx
web/nextjs/components/waflab/WafLabMatrix.tsx    The before/after grid — the point of
                                                 the whole feature. Secure vs open,
                                                 per attack, expandable to payload,
                                                 rule id, violation code, support id.
web/nextjs/components/waflab/WafLabRunsList.tsx

docs/waf-efficacy-lab-agent.md                   How it works, config table, the
                                                 honest-scope notes from §3.
```

**Edit exactly these existing files, and nothing else in the platform:**

- `server/internal/model/agent.go` — add `BuiltinWAFLab = "waf_lab"`.
- `server/internal/agentmodules/register.go` — add `WAFLab` to `Deps`, add one
  `agentmodule.Register(waflab.Module(d.WAFLab))` line.
- `server/internal/agentmodules/launch_test.go` — add a launch test in the existing style.
- `server/cmd/server` wiring — build the client and service from config, mount the handler.
- `web/nextjs/lib/agents/tab-clients.tsx` — one `waf_lab: WafLabAgentClient` entry.
- `.env.example` — the config block from §6.

If you find yourself adding a `case "waf_lab"` or an `if builtin === "waf_lab"` anywhere
in `TaskManagerPanel.tsx`, `chatagent/prompt.go`, `platformtools/execute.go` or
`tasklaunch/launch.go`, stop — you have taken a wrong turn. Those switches are existing
debt; new agents do not add to them.

---

## 5. Launch schema

One schema in `agentschema`, consumed by the web through
`GET /api/v1/agent-schemas`. Do not duplicate the fields in TypeScript.

| Key | Type | Required | Notes |
| --- | --- | --- | --- |
| `wslproxy_base_url` | text | yes | Default `https://lon1.pop0.uk`. Validate it is https. |
| `secure_host` | text | yes | e.g. `payments-secure.fictionally.org` |
| `open_host` | text | yes | e.g. `payments-open.fictionally.org` |
| `origin_upstream` | text | no | Default `127.0.0.1:30084`. |
| `policy_id` | text | no | Default `waf-policy-payments-hard`. |
| `mode` | select | yes | `provision_and_test` (default), `provision_only`, `test_only`. |
| `manage_dns` | select | no | `off` (default), `cloudflare`. `cloudflare` needs a token. |
| `dns_zone` | text | no | Only when `manage_dns=cloudflare`, e.g. `fictionally.org`. |
| `attack_set` | select | no | `full` (default), `owasp_core`, `modern_api`, `stages_only`. |
| `instruction` | textarea | no | Free note carried into the report. |

`AbsorbPrompt`: when the prompt names a host and `secure_host` is empty, fill it. Do not
guess anything else from free text.

`TitleRules`: `WAF lab: <secure_host>`. `DescRules`: mode, policy, attack set.

**Never** put a token in a schema field. Credentials come from config only (§6).

---

## 6. Configuration

Env-loaded, in the style of `simpro/client.go` `LoadConfig`. Empty base URL or empty
token disables the agent, and `Ready` reports it as a warning — same as Strix.

| Variable | Default | Meaning |
| --- | --- | --- |
| `WSLPROXY_ENABLED` | `true` | Feature flag. |
| `WSLPROXY_BASE_URL` | *(unset)* | e.g. `https://lon1.pop0.uk`. Unset disables the agent. |
| `WSLPROXY_API_TOKEN` | *(unset)* | Bearer token. Takes priority over user/password. |
| `WSLPROXY_USERNAME` / `WSLPROXY_PASSWORD` | *(unset)* | Used with `POST /api/user/login` when no token is set. |
| `WSLPROXY_PLATFORM` | `openresty-admin-next` | The `x-platform` header. See gotcha 2 in §2. |
| `WSLPROXY_PROFILE` | `prod` | The wslproxy env profile (`envProfile`). |
| `WSLPROXY_TIMEOUT` | `30s` | Bounds one HTTP call. |
| `WSLPROXY_LAB_MAX_RUNTIME` | `10m` | Backstop for a whole efficacy run. |
| `WSLPROXY_TARGET_ALLOWLIST` | *(empty)* | Defence in depth before the network hop, on top of wslproxy's own allow-list. |
| `CLOUDFLARE_API_TOKEN` | *(unset)* | Optional. Absent means the DNS phase is skipped, not failed. |
| `CLOUDFLARE_ZONE` | *(unset)* | Optional pin. |

Secrets are never logged, never returned by `Ready`, never written into a run record or
a report. Redact them in error strings too — a 401 body can echo a header.

---

## 7. Run model

The efficacy run is short (seconds to a couple of minutes), so **do not** build a Strix-
style reconciler queue. Run it inline in a goroutine with the `WSLPROXY_LAB_MAX_RUNTIME`
backstop, writing step rows as it goes, and stream progress over the existing WebSocket
the way the other agents do. Persist:

- `waf_lab_runs` — org, agent, task, status, mode, hosts, policy, counts, started/finished.
- `waf_lab_steps` — phase, status, message, timing.
- `waf_lab_results` — one row per attack per host: category, name, method, path, payload,
  status, blocked, rule id, violation, support id, latency, verdict.

Statuses mirror the pentest ones: `queued → running → completed | failed | cancelled`.
Rate-limit the relay calls (a small concurrency cap, default 4) so a full matrix does not
hammer the edge.

---

## 8. Tests

- `client_test.go` — httptest. Token refresh, the percent-encoded import body, the
  `x-platform` header, a refused non-allow-listed target, redaction on error.
- `catalogue_test.go` — parse `examples/wslproxy-waf-demo/test_waf_live.py` case list
  (path it via an env var with a skip when the repo is absent) and fail on drift.
- `score_test.go` — a fixture matrix in, expected score out, including the BOLA row
  staying "not blocked, expected" and a false positive being counted as a failure.
- `agentmodules/launch_test.go` — a `TestWAFLabLaunch` in the existing style: values in,
  output asserted, empty values return a `required` error, nil service returns
  "not configured".
- Web: a Playwright case under `web/nextjs/e2e` that loads the tab and renders a
  fixture matrix.

`go build ./... && go vet ./... && go test ./...` must pass, and
`cd web/nextjs && npm run lint && npm run build`.

---

## 9. Deployment

Runs on the `int` ring at **https://int.jobshout.co.uk**.

- Add the `WSLPROXY_*` and `CLOUDFLARE_*` variables to
  `deploy/helm/jobshout/values.yaml` and `values-int.yaml`, secrets pulled from the
  existing secret mechanism — not literals in the values file.
- Migrations run on boot (`database/migrate.go` replays every `*.up.sql`), so they must
  be idempotent. Follow `000042`.
- Update `.env.example` and `docs/waf-efficacy-lab-agent.md`.
- New orgs get the agent from `auth_service.Register` via the module `Seed`; existing
  orgs get it from the migration. Check both paths.

---

## 10. Working agreement

- Branch off the current default branch, one branch for this work. Small, reviewable
  commits.
- The wslproxy `ALLOWED_MUTATION_PLATFORMS` change is a **separate branch and PR in the
  wslproxy repo**. Do not mix the two repos in one commit.
- Do not commit or push anything until the build and the full test suite pass locally.
- Do not fire attack traffic at any host that is not on the wslproxy test allow-list, and
  do not add a host to that allow-list yourself.
- If a step is blocked (no token, no Cloudflare zone, the origin not deployed), record it
  as a skipped phase with a clear reason and carry on with the rest. A partial lab that
  says what is missing beats a run that dies at step one.

## 11. Done means

- `WAF Efficacy Lab` shows in the Task Manager rail and in chat, seeded for a fresh org
  and for an existing one, with no new switch in the platform.
- A launch against `payments-secure` / `payments-open` provisions the rules, policy,
  route and both vhosts through the wslproxy API, and is safe to run twice.
- With a Cloudflare token, the two CNAMEs appear unproxied and both hosts serve HTTPS.
- The run produces a before/after matrix in the tab UI, with rule id, violation code and
  support id per blocked attack, and a score that names the BOLA gap and any false
  positives instead of hiding them.
- `docs/waf-efficacy-lab-agent.md` explains the design, the config, and the honest limits.
- Everything builds, lints and tests green.

Start by reading `examples/wslproxy-waf-demo/README.md` and
`.claude/rules/agent-modules.md`, then write a short plan and confirm it before you
begin coding.
