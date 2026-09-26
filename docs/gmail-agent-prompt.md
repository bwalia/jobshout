# JobShout Mail Agent (Gmail) — Implementation Prompt

> Product decision (agreed): a **builtin Mail Agent** that watches a shared org
> Gmail, drafts replies (no auto-send in v1), and can commission other agents
> (especially Research) the same way Article Writer commissions Research.
>
> Paste everything below the line into the implementing agent.

---

## ROLE

You are implementing the **JobShout Mail Agent** — a builtin specialist that
connects to **one shared org Gmail**, monitors the inbox, drafts replies, and
**hands off** work that is not email (research, long-form writing, etc.) to the
existing agents.

Read this whole document before writing code. Follow the phase order. Ship each
phase as its own reviewable PR with tests that pass. Match existing JobShout
patterns; do not invent a parallel agent system.

---

## PRODUCT INTENT (non-negotiable)

**One sentence:** *A Mail Agent watches the org mailbox, drafts safe replies,
and calls other agents when the reply needs real work — human approves before
anything is sent.*

### What v1 is

| In scope | Out of scope (later) |
|---|---|
| Builtin agent on the board (`builtin: mail` / `gmail`) | Auto-send / auto-reply |
| One **shared org** Gmail via Google OAuth | Per-user personal Gmail |
| Poll inbox on a schedule + manual “sync now” | Gmail push / Pub/Sub (nice-to-have later) |
| Read threads, classify, draft reply (store as draft) | IMAP / Outlook / generic “mailbox” |
| Commission **Research Agent** when the email needs facts | Free-form multi-agent planner jobs for every mail |
| Human approve → then send | Sending without approval |
| Labels / archive after send (optional) | Full CRM / sales sequences |
| UI: connect Gmail, rules, thread list, draft review | Marketing campaigns |

### Safety rules (must hold)

1. **Draft-only by default.** The agent never calls Gmail send unless a human
   has explicitly approved that draft (API action / UI button).
2. **No silent side effects.** Labelling / archiving that mutates the mailbox
   only happens after approval of the related action, or behind an explicit
   “allow mailbox mutations” setting that defaults off except draft creation.
3. **Scopes are minimal.** Prefer Gmail scopes needed for read + draft + send
   after approval. Document every scope in code comments and UI.
4. **Org-scoped secrets.** Tokens live in encrypted / existing secrets storage
   patterns the repo already uses for integrations — never in agent metadata
   plaintext, never in logs.
5. **Commission, don’t absorb.** Research stays `ResearchService` / Research
   Agent. Mail does not reimplement search.

### OpenClaw-style composition (how agents connect)

Same mental model as Article Writer → Research:

```
Inbox event
  → Mail Agent (classify + decide)
      → optional: ResearchService.Research(...)   // typed handoff
      → draft reply (stored; not sent)
  → Human reviews draft
  → Mail Agent sends (only after approve)
```

Mail is a **capability agent**. Other agents may later call it; v1 callers are
the mail pipeline itself and (optionally) chat/platform tools that list/start
mail runs.

---

## GROUND TRUTH — COPY THESE PATTERNS

Do not redesign. Mirror:

| Pattern | Copy from |
|---|---|
| Builtin agent seed + backfill migration | `researcherSeed` in `server/internal/service/research_service.go`, migration `000021_research_agent.up.sql`, `auth_service.seedBuiltinAgents` |
| Builtin constant | `model.BuiltinResearcher` / `BuiltinArticleWriter` in `server/internal/model/agent.go` |
| Specialist package + service façade | `server/internal/research/` + `ResearchService` |
| Pipeline commissions another agent | `server/internal/blog/runner.go` → `Researcher` interface |
| Agent board activity | `model.Activity*` + blog / multi-agent attribution |
| Skills (attach later, don’t block v1) | `model.Skill`, `/skills` routes — Mail can enable skills in a later phase |
| Existing email adapter | `server/internal/integration/adapters/email` is **SMTP notifications only** — not inbox. Do not overload it for Gmail OAuth. |
| Long-running / poll work | Prefer durable rows + reconciler / scheduler (see pentest reconciler pattern) over holding HTTP open |

Migrations: `database/migrate.go` replays every `*.up.sql` on boot — **idempotent
`NOT EXISTS` guards are mandatory** (see research agent migration comments).

---

## TARGET ARCHITECTURE

```
web (Mail Agent page)
  → API /api/v1/mail/...
      → MailService
          → Gmail client (OAuth token store)
          → Mail runner / reconciler (poll → classify → draft)
          → ResearchService (optional commission)
          → AgentRepository (EnsureMailAgent, board attribution)
```

Suggested packages (names may adjust to repo style; keep boundaries clear):

| Package / area | Responsibility |
|---|---|
| `server/internal/model` | `BuiltinMail` (or `BuiltinGmail`), mail run / draft / connection types |
| `server/internal/mail` or `.../gmail` | Gmail API client, OAuth helpers, classify/draft logic |
| `server/internal/service/mail_service.go` | `MailService`: EnsureMailAgent, Connect, Sync, ListThreads, GetDraft, ApproveSend |
| `server/internal/handler` | HTTP handlers |
| `web/nextjs/.../agents/mail` (or similar) | Connect Gmail, rules, thread + draft review UI |
| Migration(s) | Seed builtin; tables for connection + runs + drafts |

### Typed handoffs (v1)

Define small, explicit types — do not pass free-form “ask the other agent” blobs
as the only contract:

- `mail.ClassifyResult` — intent, needs_research bool, urgency, suggested action
- `mail.Draft` — thread_id, body, subject, to/cc, research_brief_id (optional), status (`draft` \| `approved` \| `sent` \| `rejected`)
- Research: reuse `research.Request` / `research.Brief` when commissioning

---

## PHASES

### Phase 0 — Spec lock (no feature code yet)

- Add this doc’s product intent to any short ADR note only if the team requires
  it; otherwise keep this file as the source of truth.
- Confirm Google Cloud OAuth client setup is an ops prerequisite (document env
  vars: client id/secret, redirect URL, token encryption key if needed).

**Done when:** implementer can list env vars and tables they will add.

### Phase 1 — Builtin Mail Agent + empty service

- Add `BuiltinMail` (name in UI: **Mail Agent**, role e.g. `Mail`).
- Seed in `auth_service.seedBuiltinAgents` and idempotent backfill migration
  (mirror research).
- `EnsureMailAgent(ctx, orgID)`.
- Agent appears on agent board as idle/active like other builtins.
- Stub `MailService.Available()` false until Gmail connected.

**Done when:** new + existing orgs show Mail Agent; tests for seed/ensure.

### Phase 2 — Gmail OAuth (org shared mailbox)

- Connect / disconnect / status endpoints.
- Store refresh token per org (one connection in v1).
- UI: “Connect Gmail” → OAuth → show connected account email.
- Never log tokens. Redact in error messages.

**Done when:** org can connect Gmail in UI; API can obtain a valid access token
for Gmail API calls in tests with a fake/mock.

### Phase 3 — Monitor (read-only) + durable runs

- Poll unread / recent messages on an interval (config) and “Sync now”.
- Persist mail runs / thread snapshots needed for the UI (enough to show subject,
  from, snippet, status).
- Classify with LLM (structured JSON): triage label, whether research is needed,
  one-line reason.
- Board activity: e.g. monitoring / executing while syncing.

**Done when:** connected org sees new threads in UI after sync; classification
stored; unit tests for classifier with fixture emails.

### Phase 4 — Draft replies + Research handoff

- For threads that need a reply: produce a **draft** (DB + optionally Gmail
  draft create — prefer DB-first so approval is always in JobShout).
- If `needs_research`: call `ResearchService.Research` with a brief derived from
  the email; fold findings into the draft; attribute research to Research Agent
  on the board (same spirit as blog).
- Draft UI: show original thread, research summary (if any), editable draft body.
- **No send in this phase** (or send endpoint returns 403 / not implemented).

**Done when:** end-to-end: sync → classify → research (when needed) → draft
visible; Article-Writer-style commission path covered by a service test with
fakes.

### Phase 5 — Human approve → send

- `POST .../drafts/{id}/approve` (or approve + send).
- Only then call Gmail send (and optional label/archive if enabled).
- Audit fields: who approved, when, gmail message id.
- Reject path leaves draft rejected and does not send.

**Done when:** send cannot happen without approve; tests prove the guard.

### Phase 6 — Polish (only if Phases 1–5 are solid)

- Simple rules: only labels / senders / subject prefixes to watch.
- Chat/platform tool: list mail drafts pending approval / trigger sync (optional).
- Skill hook: allow enabling a “brand voice” skill on Mail Agent (optional).
- Docs page similar to `docs/pentest-agent.md` for operators.

---

## API SKETCH (adjust to existing handler style)

Prefer under `/api/v1/mail` (or `/api/v1/agents/mail`):

| Method | Path | Purpose |
|---|---|---|
| GET | `/connection` | Status + connected email |
| POST | `/connection/oauth/start` | Returns auth URL |
| GET | `/connection/oauth/callback` | Exchanges code (or POST if SPA-style) |
| DELETE | `/connection` | Disconnect |
| POST | `/sync` | Sync now |
| GET | `/threads` | List watched threads / runs |
| GET | `/threads/{id}` | Thread + classification + draft |
| PATCH | `/drafts/{id}` | Edit draft body before approve |
| POST | `/drafts/{id}/approve` | Approve and send |
| POST | `/drafts/{id}/reject` | Reject |

AuthZ: org membership + same patterns as other agent routes.

---

## UI SKETCH

Mirror Security Tester / Articles agent pages (app chrome: sidebar + topbar):

1. **Setup** — Connect Gmail, show account, disconnect.
2. **Inbox / runs** — list with status: new / researching / draft_ready / sent / rejected.
3. **Thread detail** — message, classification, research brief link/summary, draft editor, Approve / Reject.

Copy tone: same plain language as Article Writer (“Mail Agent is working on it”).

---

## TESTING REQUIREMENTS

- Unit: classifier + draft prompt building with fixture emails (no network).
- Service: ApproveSend refuses when status ≠ draft/approved-pending; Research
  commissioned only when flagged.
- Seed migration: re-run safe (idempotent).
- OAuth / Gmail client behind interfaces; fakes in tests.
- Do not require real Gmail in CI.

---

## EXPLICIT NON-GOALS FOR THIS PROMPT

- Auto-reply / auto-send.
- Per-user OAuth.
- Replacing SMTP notification adapter.
- Building a general “OpenClaw runtime” or new multi-agent framework — **reuse**
  ResearchService + builtin agents + durable runs.
- Verbatim copying of third-party agent products; match JobShout’s Article
  Writer / Research pattern only.

---

## ACCEPTANCE CHECKLIST (v1 complete)

- [ ] Mail Agent seeded for new and existing orgs (`builtin` metadata set).
- [ ] Org can connect one shared Gmail and see connection status.
- [ ] Sync brings threads into JobShout; classification stored.
- [ ] Drafts created without sending.
- [ ] Research Agent commissioned when classification says research is needed.
- [ ] Approve sends; reject does not; no path sends without approve.
- [ ] Tokens never logged; scopes documented.
- [ ] Agent board shows Mail (and Research when commissioned) meaningfully.
- [ ] Tests cover seed, classify, research handoff guard, send guard.

---

## IMPLEMENTATION NOTES FOR THE AGENT

1. Prefer small PRs per phase; do not merge auto-send “while we’re here.”
2. If stuck on Google OAuth UX, finish Phase 1 + interfaces + fakes first so
   the pipeline is testable offline.
3. When commissioning research, attribute work like blog does so the board
   does not lie.
4. Keep system prompt of the Mail Agent short and strict: triage, draft, never
   claim to have sent unless the send API succeeded after approval.
5. If a choice conflicts with this doc vs existing code style, **follow existing
   code style** but do not violate the safety rules above.

---

## ONE-LINE REMINDER

**Mail Agent = inbox specialist + draft-only + Research handoff + human send.
Not a mega-agent. Not auto-reply.**
