# JobShout iOS

The iOS app is the mobile front door to the whole ecosystem: Showcase, Agents, Work, Jobs, Insights. The loop that matters is **Showcase → Agent → Call → Schedule → Work → Approve → Result**. Jobs and builders feed into it.

This doc maps the product brief (60 sections) onto what the two backends actually have, fixes the architecture decisions, and cuts a v1.

## The brief's assumptions vs the repo

| Brief says | Repo has |
|---|---|
| One backend on Rust/Axum | **Two.** Go platform (`server/`, `jobshout.co.uk`) owns identity, agents, tasks, executions, schedules, approvals, budgets, chat, career. Rust (`jobshout-com/`, `jobshout.com`) owns public jobs, profiles, Showcase apps, Insights. |
| One identity | Go: HS256 JWT (15 min) + rotating opaque refresh token (7 d). Rust: no bearer auth; writes trust `x-jobshout-user-email` signed with the shared `x-jobshout-internal-token` (only Next.js server can send it). |
| OpenAPI-generated client | No spec in Go. Rust declares `utoipa` but doesn't use it. |
| WebSocket live status | `GET /api/v1/ws` exists, but **nothing calls `Hub.BroadcastToOrg`**, and `/ws` runs under the 30 s `requestTimeout`, so the socket's context dies at 30 s. |
| Push notifications | None. No device table, no APNs. Notification configs are Slack/Teams/Telegram. |
| Execution lifecycle state machine | Executions: pending/running/completed/failed/cancelled. Tasks: backlog/todo/in_progress/review/done (not enforced). Cancel is per-specialist only. |
| Teams (humans + agents) | No teams model in either backend. |
| Placeholder `jobshout-com/ios/` | README only, scoped to the Rust candidate API. Superseded by this plan. |

## Decisions

1. **Go is the app's only origin.** The app talks to `https://<ring>.jobshout.co.uk/api/v1` with one bearer token. Go proxies the Rust reads/writes it needs (`/api/v1/com/...` → `JOBSHOUT_COM_API_URL`), signing with the internal token and the authenticated user's email, exactly as the Next.js server does today. One host, one auth, one spec, one place for rate limits and audit. Later, if Rust needs to verify tokens itself, Go moves to ES256 + JWKS; not needed for v1.
2. **Sessions become device-bound.** `refresh_tokens` gains `device_id`; a `devices` table holds platform, APNs token, app version, last seen. Logout revokes one device; "log out everywhere" revokes all.
3. **Sign in with Apple is mandatory**, not optional: App Store 4.8 requires it once Google sign-in is offered. Native flow: app sends the Apple identity token + nonce; Go verifies against Apple's JWKS. Google moves to the same shape (ID-token exchange), since the web redirect flow hard-codes `FRONTEND_BASE_URL`.
4. **Hand-written OpenAPI, guarded by a test.** `server/api/openapi.yaml` covers the mobile surface only. A Go test walks the chi router and fails if a spec path isn't registered. The Swift client is generated from it with `swift-openapi-generator` at build time, so there's no generated code to commit.
5. **Agent launch stays generic.** The app renders launch forms from `GET /agent-schemas` (`WireSchema.fields`) and calls `POST /tasks/launch`. There's no per-agent Swift code; that's the same rule as `.claude/rules/agent-modules.md`. An agent tab UI (`AGENT_CLIENTS`) is web-only; on iOS, every agent gets the schema form.
6. **Live work: WebSocket for events, polling as truth.** Events trigger a refetch; they never become state on their own. Missed events cost latency, not correctness. The app shows "last synced" whenever it's offline.
7. **No chain-of-thought on the device.** Live activity shows tool calls, status transitions and user-facing progress only: the same events chat already streams (`tool_call`, `tool_result`, `confirmation`).
8. **Project shape.** `ios/` at the repo root (it spans both backends). `JobShoutKit` is a local Swift package with the modules below. The app target is generated with XcodeGen (`project.yml`), so no `.pbxproj` is committed. iOS 18 minimum, Swift 6 strict concurrency, `@Observable`.

```
ios/
├── project.yml                 XcodeGen: app, widgets, intents, share extension
├── App/                        @main, tab shell, deep-link router, DI container
├── JobShoutKit/                Swift package
│   ├── Core                    Environment (int/test/acc/prod), logging (no secrets), Clock
│   ├── Networking              APIClient, auth middleware, retry on 401 → refresh, SSE-over-POST, WebSocket
│   ├── Auth                    Keychain token store, session actor, Apple/Google/email, biometric unlock
│   ├── Storage                 Offline cache with synced-at stamps
│   ├── DesignSystem            Tokens, cards, status pills, empty/error states
│   └── Features/…              Home, Showcase, Agents, Work, Jobs, Insights, Search, Profile
└── JobShoutKitTests/
```

## Brief → status → phase

✅ exists and is usable as-is · 🟡 exists, needs work · ❌ missing

| § | Area | Backend today | Phase |
|---|---|---|---|
| 4 | Email/password auth, refresh | ✅ `/auth/login`, `/register`, `/refresh`, `/me` | 1 |
| 4 | Sign in with Apple, native Google, logout, logout-all, device sessions | ❌ | 1 |
| 4 | Passkeys, MFA, enterprise SSO | ❌ (org SSO exists, web-only) | 4 |
| 5 | Onboarding, multi-role profile | 🟡 single `role` on user; no persona roles | 2 |
| 7 | Home command centre | 🟡 compose from `/agents/board`, `/approvals`, `/scheduled-tasks`, `/tasks` | 2 |
| 8–9 | Showcase browse + detail | ✅ Rust `/showcase/apps*` (proxy) | 2 |
| 10–11 | Call Agent | ✅ `/agent-schemas` + `/tasks/launch`; 🟡 `/marketplace` is auth-only, no public listing | 1 |
| 12–13 | Schedule agents | 🟡 `/scheduled-tasks` CRUD; ❌ timezone, run-now; pause = `PUT status` | 2 |
| 14–15 | Work centre, live activity | 🟡 tasks/runs/executions by polling; ❌ WS broadcasts; ❌ generic cancel | 1 (poll) / 2 (live) |
| 16 | Approvals | ✅ `/approvals`, `/approvals/{id}/decide`; 🟡 no signed approval record | 1 |
| 17–18 | Human + AI teams, agent teams | ❌ no teams model (multi-agent workflows exist) | 3 |
| 19–22 | Jobs, agent-compatible work, Apply with AI | 🟡 Rust `/jobs` (no auth); Go `/career/*` (apply is dry-run only, by design) | 2 |
| 23–24, 44–46 | Builder funnel, showcase import, creator dashboard, verification | 🟡 Rust showcase create/mine/review-queue | 3 |
| 25–26 | Insights + Ask AI | ✅ Rust `/insights*` public reads; Ask AI via chat with article context | 2 |
| 27 | Push | ❌ devices + APNs sender | 1 (register) / 2 (send) |
| 28–29 | Widgets, App Intents | ❌ (client-only, on top of phase 1–2 APIs) | 3 |
| 30–31 | Universal / semantic search | ❌ no cross-entity search endpoint | 3 |
| 32–35 | Marketplace signals, permissions, cost, reliability | 🟡 selector has `success_rate`, `total_runs`; budgets/policies exist; ❌ pre-run estimate, ❌ per-agent tool permission listing | 2–3 |
| 36 | Audit trail | 🟡 `/audit/actions` exists but only agent-pack import writes to it | 2 |
| 38 | Universal links | ❌ needs `apple-app-site-association` on both hosts | 2 |
| 39–40 | Offline, background refresh | client-only | 2 |
| 41–43 | Voice, camera/files, share extension | 🟡 CV upload; ❌ malware scanning on uploads | 3 |
| 47 | Monetisation | ❌ | 4 |
| 52 | Chat | ✅ SSE-over-POST `/chat/sessions/{id}/messages/stream` with confirmation events | 2 |
| 56–58 | App Store, tests, CI | ❌ | 1 (CI) / 4 (store) |

## Phases

**Phase 1: foundations + the core loop (TestFlight internal).**
Backend: logout/revoke, device registration, Sign in with Apple, `/ws` timeout exemption + real broadcasts (task transition, execution status, approval requested), OpenAPI for the mobile surface with a route-drift test.
App: sign in (Apple / email), Agents list → agent detail → schema-driven **Call Agent** → Work list (polled) → Approvals inbox → approve/reject. CI builds and unit-tests the package on every PR touching `ios/`.

**Phase 2: the ecosystem (TestFlight external).**
Go proxy to Rust for Showcase, Insights and Jobs. Home dashboard, schedules (timezone + run-now on the backend), live Work over WebSocket, APNs sender (completed work, approval needed), chat with confirmations, Career Agent evaluate/apply-prep, universal links, offline cache, audit writes for launch/approve/schedule.

**Phase 3: builders and teams.**
Teams model (humans + agents), creator dashboard + showcase import (GitHub URL → draft → human publishes), search endpoint, widgets, App Intents with confirmation, share extension, voice, upload scanning.

**Phase 4: store and scale.**
Passkeys, MFA, SSO; monetisation (StoreKit behind a server entitlement API, never UI-coupled); privacy manifest from real data flows; screenshots; account deletion (store requirement: must ship before public release, so it starts in phase 2 backend work).

## v1 definition of done (end of phase 2)

A user can sign in with Apple, browse Showcase and Insights, find an agent, see its launch fields, run it now or on a schedule, watch the run, get a push when it needs approval, approve it from the notification, and see the result. Every step goes through Go with the user's token; no business logic lives in Swift.

## Not doing

- Executing agent code on the device. All execution is server-side.
- Submitting job applications automatically. `career/apply` stays dry-run; the app shows the package for review.
- Inventing reliability numbers. Stats come from recorded executions only, with the sample size shown.
- A WebView shell.

## Implementation order (PRs)

1. This plan.
2. Backend prerequisites (phase 1 backend): logout + devices + Apple + WS + OpenAPI.
3. `ios/` scaffold: package modules, Keychain session, API client, tab shell, sign in, Agents → Call Agent → Work → Approvals against int, plus a GitHub Actions `ios` job.
