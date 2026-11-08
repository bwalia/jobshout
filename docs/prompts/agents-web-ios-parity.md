# JobShout Agents — Web UX + iOS Feature Parity

> Paste everything below the line into the implementing agent (or use this file as the
> task brief). **Do not invent a new product look.** Follow existing JobShout web patterns
> and the agent-modules rule. Prompt only — this document is the work order, not the
> implementation.

---

## ROLE

You are improving **JobShout Agents** so the web panel is easy to navigate and the
**iOS app** can do the same agent work a user does on web: find agents, understand them,
launch them, watch work, approve, schedule, and talk to agents from chat where that
already exists on web.

Read this whole document before writing code. Prefer small reviewable PRs in the phase
order below. Reuse existing APIs (`GET /api/v1/agent-schemas`, `POST /api/v1/tasks/launch`,
OpenAPI-generated Swift client). Do **not** special-case new specialists in platform
launch/chat code — see Constraints.

---

## GOAL / SUCCESS CRITERIA

1. **Web navigation:** A signed-in user on int can go from sidebar → any agent → primary
   action (Run / New task / Workspace / Chat) in **≤2 clicks**, without hunting across
   duplicate “Agents / Task Manager / Chat / Tasks” mental models.
2. **iOS parity (Agents core loop):** On iPhone (Debug → int), a user can: browse/search
   agents → open agent detail with the same *decision-critical* info as web Overview →
   launch via schema form → land on the task/run → approve when needed → see schedules
   and (where APIs exist) create/pause them → start or continue **chat** that can drive
   agents.
3. **Naming consistency:** Web “Run” / “New task” / “Workspace” and iOS “Call agent” /
   Work map to the same server operations; labels should not invent a third vocabulary
   without reason (prefer aligning iOS copy toward Run / New task if that reduces
   confusion).
4. **No per-agent Swift UIs** for Meta Lead Analyst, Article Writer, etc. Specialist
   *tab clients* stay web (`AGENT_CLIENTS`). iOS keeps schema-driven launch; deep results
   UIs either open web (SFSafari / ASWebAuthenticationSession / in-app WKWebView to
   int panel) **or** a generic “runs / artifacts / links” surface — never a new
   `if builtin == "meta_lead_analyst"` branch in platform launch code.
5. **Verify on int:** Web `https://int.jobshout.co.uk` and iOS Debug against
   `https://int.jobshout.co.uk/api/v1`.

---

## CURRENT STATE SUMMARY (verified against repo)

### Web (Next.js — `web/nextjs`)

| Surface | Route / entry | What it does |
|---|---|---|
| All agents | `/panel/agents` (`app/(app)/panel/agents/page.tsx`) | Searchable list; Create; Import; last-run cells; row → profile |
| Agent profile | `/panel/agents/[id]?tab=` (`…/agents/[id]/page.tsx`) | Tabs: **overview · models · workspace · tasks · metrics · knowledge · skills** |
| Workspace | `?tab=workspace` | If `metadata.builtin` ∈ `AGENT_CLIENTS` → specialist UI; else schema form + Run (`BuiltinAgentTab`) |
| Run | Header **Run** → `useUiStore.openRunDialog` → `RunTaskDialog` + `AgentInputFields` | `POST /tasks/launch` via `lib/agents/launch.ts` |
| New task | `/panel/tasks?new=1&agent=<id>` (`newTaskHref`) | Board task with agent preselected |
| Chat | `/chat` (`ChatPage`, `ChatSidebar`) | Natural-language drive; specialists via `agent_execute` + schema interview |
| Nav data | `lib/nav.ts` | Agents section: All agents, Org chart, Skills & plugins, Marketplace; `NEW_TASK_HREF` |
| Panel registry | `lib/panels.ts` | Parallel IA (chat-first panels); agents/task-manager often `hidden` |
| Specialist clients | `lib/agents/tab-clients.tsx` + `lib/agents/specialist.ts` | e.g. `meta_lead_analyst` → `MetaLeadClient`; also article_writer, mail, pentester, … |
| Schemas | `GET /api/v1/agent-schemas` (`lib/api/agent-schemas.ts`) | Single source for launch fields (web + iOS) |

**Web UX pain points visible in structure**

- **Two nav models:** `lib/nav.ts` (sidebar sections) and `lib/panels.ts` (chat-first panel
  list) still coexist; agents appear “hidden” in panels while also owning a full Agents
  section — easy to feel lost.
- **Seven profile tabs** on `/panel/agents/[id]` — Overview / Models / Workspace / Tasks /
  Metrics / Knowledge / Skills compete; primary action (Run) is header-only while
  Workspace sometimes *is* the product (Meta Lead, Articles).
- **Three launch paths** (Run dialog, New task, Chat, plus Workspace schema form for
  agents without a client) — powerful but cognitively heavy; labels and destinations
  should be clarified, not duplicated further.
- Legacy redirects (`/panel/task-manager`, `/panel/task-board`, security-tester) still
  exist; keep them working but stop teaching them as primary IA.
- Command palette (`CommandPalette`) already knows “Run an agent…” / “New chat” — lean on
  it rather than adding more top-level destinations.

### iOS (`ios/` — JobShoutKit)

| Surface | File | What it does |
|---|---|---|
| Tabs | `RootView.swift` → `MainTabView` | Home · Agents · Projects (freelance catalog) · Work · Schedules · Approvals · Me |
| Agents list | `AgentsView.swift` | Search name/role/description → `AgentRoute` |
| Agent detail | `AgentDetailView` in same file | Avatar, role, status, engine/model/builtin; **Call agent** sheet |
| Launch | `CallAgentView.swift` + `LaunchForm.swift` | Schema from `PlatformAPI.launchSchema` → `POST` launch |
| Work | `WorkView.swift` / `TaskDetailView` | Filter by status; runs list; live revision via `AppModel` |
| Approvals | `ApprovalsView.swift` | Pending badge; decide approve/reject |
| Schedules | `SchedulesView.swift` | List + pause/resume only; empty state says create on web |
| API | `PlatformAPI.swift` + OpenAPI client from `server/api/openapi.yaml` | agents, schemas, projects, tasks/launch, runs, approvals, schedules, devices, WS |
| Plan / TF notes | `docs/plans/08-ios-app.md`, `ios/Brand/TESTFLIGHT.md`, `ios/README.md` | Explicit: **agent tab UIs are web-only**; chat / Showcase / APNs delivery not in current TF track |

**iOS gaps vs web (must-have Agents parity)**

| Capability | Web | iOS today |
|---|---|---|
| List + search agents | ✅ `/panel/agents` | ✅ `AgentsView` |
| Schema launch (`/agent-schemas` + `/tasks/launch`) | ✅ | ✅ `CallAgentView` |
| Watch task / runs | ✅ Tasks + Runs panels | ✅ Work tab |
| Approvals | ✅ (chat cards + APIs) | ✅ Approvals tab |
| Agent profile depth (models, knowledge, skills, metrics, failed-run banner) | ✅ | ❌ detail is thin |
| Specialist Workspace product UI (Meta Lead, Articles, …) | ✅ `AGENT_CLIENTS` | ❌ by design — need generic results / web handoff |
| Chat with agents | ✅ `/chat` | ❌ not in app; not in OpenAPI mobile surface yet |
| Create / import / export / remove agent | ✅ | ❌ |
| Edit model / skills / knowledge | ✅ | ❌ |
| Create schedule (not only pause) | ✅ `/panel/scheduler` | ❌ list + status only |
| New task on board (vs immediate Run) | ✅ | ❌ launch is run-now only |
| Org chart / Marketplace / Plugins | ✅ nav | ❌ out of core loop (see Out of scope) |
| “Projects” meaning | JobShout workspace projects | iOS tab is **freelance catalog** (`FreelanceProjectsView`) — naming collision risk |

---

## UX PRINCIPLES (Agents navigation)

1. **One primary path to “make an agent work”.** Prefer: Agents list → agent → **Run**
   (schema) *or* **Workspace** (if client) *or* **Ask in chat**. Secondary: New task
   (board), Schedules.
2. **≤2 clicks** from Agents entry to the action that starts work.
3. **Brand-consistent, not AI-slop.** Follow `docs/brand/BRAND.md` and existing web tokens
   (Sora / existing Tailwind theme, status colours, hairline borders). Do **not** invent a
   purple-gradient / glow / cream-serif redesign. iOS should stay native SwiftUI with
   existing `StatusPill` / accent patterns.
4. **Schema once, everywhere.** Launch fields come only from `GET /agent-schemas` (or
   generic). Web: `AgentInputFields`. iOS: `LaunchForm` / `SchemaFieldView`. No duplicated
   field lists in TS and Swift.
5. **Specialist screens register, they don’t fork the platform.** Web tab clients only in
   `AGENT_CLIENTS`. iOS must not grow per-builtin view switches for launch.
6. **Clear location.** Active nav item, agent name, and current tab always visible; failed
   runs stay visible (web already has failed banner + `failed-agents` badge).
7. **Mobile honesty.** Heavy specialist consoles (Meta Lead CRM push, article workstation)
   may deep-link to int web Workspace; the phone must still **launch, monitor, approve**.

---

## FEATURE PARITY MATRIX (must-have)

Mark each row done only when **web UX is clear** *and* **iOS can complete the user job**
(or an explicit, documented web-handoff for specialist-only UI).

| ID | User job | Web target | iOS target | APIs |
|---|---|---|---|---|
| P1 | Find an agent | `/panel/agents` scannable; search; status/last-run | Agents tab search; status pill; optional last-run if API cheap | `GET /agents` |
| P2 | Understand agent | Overview + obvious Run / Workspace CTA | Richer detail: role, description, builtin, model, deep link to Workspace on web when client exists | `GET /agents/{id}` |
| P3 | Run now | Run dialog + Workspace schema Run | Call/Run sheet (`CallAgentView`) → task | `GET /agent-schemas*`, `POST /tasks/launch` |
| P4 | Track work | Task detail / Runs | Work → TaskDetail → runs | `GET /tasks`, `/tasks/{id}/runs` |
| P5 | Approve | Chat confirmation + approvals UX | Approvals tab (already) | `/approvals`, `/decide` |
| P6 | Schedule | Scheduler create + pause | List + pause **and** create/edit enough fields for “run this agent later” | `/scheduled-tasks` |
| P7 | Chat with agents | `/chat` polished entry from agent profile (“Ask in chat”) | Chat tab or Home entry; sessions + stream | extend OpenAPI + Swift for chat SSE (see `docs/plans/08-ios-app.md` §52) |
| P8 | See specialist results | Workspace `AGENT_CLIENTS` (e.g. Meta Lead) | Generic “Latest runs / open on web” — **no** MetaLead Swift clone | existing run APIs + optional web URL to `/panel/agents/{id}?tab=workspace` |
| P9 | Models (read + change) | Models tab | Read model; change if `PATCH /agents/{id}` already supports (else read-only + web link) | agent update endpoints already used by web hooks |
| P10 | Knowledge / Skills (read) | Knowledge + Skills tabs | At least list enablement; full editors can stay web if timeboxed | knowledge/skills APIs used by web |

**Stretch (same release train if cheap, else phase 3):** Create/import agent on iOS;
export package; agent metrics charts; org-builder / marketplace.

---

## SUGGESTED PHASED PLAN

### Phase A — Web Agents UX (first)

Ship navigation clarity **before** large iOS surface area.

1. Audit Agents IA: sidebar (`lib/nav.ts`) vs panels (`lib/panels.ts`) vs ChatSidebar CTAs;
   collapse duplicate teaching (“Task Manager” vs “All agents”).
2. Agent profile: promote **one** primary CTA row (Run · Workspace · Ask in chat · New
   task); demote or group Models / Knowledge / Skills / Metrics so Workspace and Tasks
   aren’t equal peers for every agent.
3. For builtins with `AGENT_CLIENTS`, default landing tab can be **workspace** (or a
   clear “Open workspace” that isn’t a tiny chip only).
4. Agents list: keep search + failure signals; ensure empty/error states point to Chat or
   New agent once.
5. E2E: extend `web/nextjs/e2e/agents.spec.ts` / `navigation.spec.ts` for the new primary
   path.

### Phase B — iOS Agents parity (core loop+)

1. **Agent detail upgrade** to match P2/P9/P10 read paths; align copy (Run vs Call).
2. **Chat** on iOS against int (OpenAPI + generated client + SSE). Reuse server chat;
   don’t fork prompts.
3. **Schedules create** (minimal fields) + existing pause/resume.
4. **Specialist handoff:** if `builtin` has a web client, show “Open workspace on web”
   with authenticated or copy-link flow to int.
5. Optional: “New task” (create board task without immediate run) if product needs board
   parity.
6. Update `ios/UITests/CoreLoopUITests.swift` and `ios/Brand/TESTFLIGHT.md` “what this
   build covers”.

### Phase C — Parallel polish

- Live WS reliability for Work (already planned in `08-ios-app.md`).
- Push when approval needed (APNs) — backend may still be incomplete; don’t block A/B.
- Resolve **Projects** naming collision (freelance vs workspace projects) in iOS tab
  label/copy.

**Recommended order:** **A → B → C**. Parallelize only if two people: one owns web IA,
one owns iOS chat/OpenAPI — share the parity matrix IDs in PR titles.

---

## CONSTRAINTS

1. **Agent modules rule** (`.cursor/rules/agent-modules.mdc`, `.claude/rules/agent-modules.md`):
   - No new platform `if` for specialists in launch/chat execute paths.
   - New specialists: seed + schema + optional `AGENT_CLIENTS` entry + chat tools with the
     agent package.
   - Do not reintroduce `TaskManagerPanel` BUILTINS / duplicated `input-schemas` cases.
2. **One schema:** Go `agentschema` → `GET /api/v1/agent-schemas`; web and iOS consume it.
3. **API reuse:** Prefer existing Go routes; extend `server/api/openapi.yaml` when iOS
   needs chat/knowledge so `openapi_test.go` stays green and Swift regenerates.
4. **No credentials in launch schema fields** — server-side creds by name (CLAUDE.md).
5. **iOS shipping:** XcodeGen (`ios/project.yml`); Debug → int; TestFlight notes in
   `ios/Brand/TESTFLIGHT.md` (ASC app `6829933699`, bundle `com.jobshout.app` for Release).
   Bump `CURRENT_PROJECT_VERSION` on upload; never commit `DEVELOPMENT_TEAM`.
6. **Web CI:** `cd web/nextjs && npm run typecheck && npm run lint` before push.
7. **Migrations:** if any, `IF NOT EXISTS` / next free number from `origin/master`.
8. **Do not** wrap the iOS app in a full WebView shell (explicit non-goal in
   `docs/plans/08-ios-app.md`). Targeted web handoff for specialist Workspace is OK.

---

## OUT OF SCOPE

- Redesigning the entire JobShout marketing site or jobshout.com.
- Cloning every `AGENT_CLIENTS` screen into Swift (Meta Lead CRM, Article Writer
  workstation, Pentest console, Video producer, …).
- Watch / Mac feature parity beyond what already shares JobShoutKit (focus = **iPhone
  Agents** + web).
- Marketplace / Org builder / Plugins full iOS ports.
- Monetisation, passkeys, MFA.
- Executing agents on-device.
- Inventing new purple/AI aesthetic or replacing brand fonts/colours.

---

## ACCEPTANCE TESTS / HOW TO VERIFY

### Web (int)

1. Sign in at `https://int.jobshout.co.uk`.
2. Sidebar → **All agents** → open **Meta Lead Analyst** (or any `AGENT_CLIENTS` agent):
   Workspace is obvious; Run / New task / Ask in chat reachable without leaving the
   profile maze.
3. Open a custom (non-client) agent: Workspace shows schema + Run; launch creates a task
   visible under Tasks / Runs.
4. From Chat, ask to run a known specialist; confirm schema interview still works (no
   regressions from IA changes).
5. Command palette: “Run an agent…” and “New chat” still work.
6. `npm run typecheck && npm run lint` in `web/nextjs`; agents e2e green.

### iOS (simulator / device → int)

1. Scheme **JobShout**, Debug, team set; footer shows Integration
   (`ios/README.md`). Prefer Google on int.
2. Home → Agents → search → open agent → **Run/Call** → fill schema → Run now → lands on
   task with runs.
3. Approvals: if a run requests approval, decide from Approvals tab.
4. Schedules: pause/resume; **create** a schedule for an agent (Phase B) and see it listed.
5. Chat (Phase B): start a session, send a message, see streaming reply; trigger an agent
   from chat if server supports it.
6. Specialist with web client: “Open workspace” opens int agent Workspace (or copy link).
7. `cd ios/JobShoutKit && swift test`; optional UITest core loop with
   `TEST_RUNNER_JOBSHOUT_UITEST_*` env vars.
8. Update TestFlight “what this build covers” when shipping a build that includes chat /
   schedules create.

### Parity checklist (PR description)

Copy the matrix IDs **P1–P10** into the PR and tick web + iOS columns. Any intentional
web-only cell must say “web handoff” with the exact URL pattern
`/panel/agents/{id}?tab=workspace`.

---

## KEY FILES TO READ FIRST

**Web:** `web/nextjs/app/(app)/panel/agents/page.tsx`,
`…/agents/[id]/page.tsx`, `web/nextjs/lib/agents/tab-clients.tsx`,
`web/nextjs/lib/agents/links.ts`, `web/nextjs/lib/nav.ts`, `web/nextjs/lib/panels.ts`,
`web/nextjs/components/task-manager/BuiltinAgentTab.tsx`,
`web/nextjs/components/task-manager/RunTaskDialog.tsx`,
`web/nextjs/components/layout/ChatSidebar.tsx`,
`web/nextjs/components/meta-leads/MetaLeadClient.tsx`

**iOS:** `ios/JobShoutKit/Sources/JobShoutFeatures/Screens/{RootView,AgentsView,CallAgentView,WorkView,HomeView,SchedulesView,ApprovalsView}.swift`,
`…/Domain/{PlatformAPI,LaunchForm}.swift`, `ios/Brand/TESTFLIGHT.md`,
`docs/plans/08-ios-app.md`, `server/api/openapi.yaml`

**Rules:** `.claude/rules/agent-modules.md`, `docs/brand/BRAND.md`, `CLAUDE.md` (CI, rings,
secrets)

---

## DELIVERABLES

1. Web UX PR(s) for Phase A.
2. iOS + OpenAPI PR(s) for Phase B (chat + richer agent detail + schedules create +
   specialist web handoff).
3. Short note in `ios/Brand/TESTFLIGHT.md` and optionally a bullet in
   `docs/plans/08-ios-app.md` pointing at completed parity IDs — **no** new parallel plan
   doc unless the matrix changes substantially.
