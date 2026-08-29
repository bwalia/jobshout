# Plan 3 — Research Agent

> ## Execution status — 2026-08-29
>
> **Done: phases 1–3, plus evals.**
>
> - Migration `000035_research_runs` (idempotent — `migrate.go` replays every
>   `.up.sql` on boot), `model.ResearchRun`, `repository.ResearchRunRepository`.
> - `researchService.Run` records every research call; `Research` delegates to
>   it, so all four entry points now leave a row. Bookkeeping is best-effort by
>   design: an unwritable run row degrades to the old behaviour rather than
>   losing a completed brief, and there is a check asserting exactly that.
> - `StartAsync` + `POST/GET /api/v1/research/runs` — the pollable form.
>   `POST /research` keeps its synchronous contract so no caller breaks.
> - Board: a `research_activity` arm was added to the union in
>   `multi_agent_repository.go`, with `boardActivity` mapping research statuses.
>   **The Research Agent can appear on the board for the first time.**
> - Evals at `server/eval/research/` — 20 checks, green: happy path, unusable
>   sources failing honestly, pinned URLs read rather than searched, and
>   bookkeeping never failing the work.
>
> **Deviation from this plan, deliberately.** The plan preferred renaming
> `mail_threads.research_brief_id` to `research_run_id`. Measuring the blast
> radius changed the answer: ~20 SQL sites in `mail_repository.go`, two model
> structs, and the public `research_brief_id` JSON field. The goal was that the
> identifier *resolves*, not what it is called — so the column keeps its name
> and now holds a real `research_runs.id`. No FK was added either: pre-existing
> rows hold invented UUIDs, so even a `NOT VALID` constraint would be a trap for
> whoever backfills them. Both decisions are recorded in the migration.
>
> **Outstanding:** phase 4 (surfacing `usable = false` in the UI) — the flag is
> now on the run and exposed by the API, but nothing renders it yet. `launch.ts`
> still uses the synchronous path; moving it to polling belongs with plan 4.

Verified against `feat/landing-page` @ `063cce3`.

The brief says "believed to function correctly; run a small set of evaluations
to confirm." That reading is right about the logic and misses the real defect:
**the Research Agent has no persistence at all.**

---

## What is actually in the repo

`server/internal/research/` — 248K, the most thoroughly built package in the
server, and it does real work rather than asking a model to pretend:

| File | Role |
|---|---|
| `agent.go` | orchestration; phases `Planning → Searching → Reading → Verifying` |
| `discover.go` | query planning and candidate discovery |
| `verify.go` | checks each claim against the source it cites |
| `excerpt.go` | pulls supporting text out of fetched pages |
| `focus.go` | topic focusing |
| `arxiv.go`, `github.go`, `hackernews.go`, `feeds.go` | source backends |
| `jina.go` | page fetch/read |
| `client.go` | HTTP |

Design details worth preserving:

- `Brief.IsUsable` (`agent.go:61`) reports whether enough verified material came
  back, with `MinFindings` as the floor (`:101`). The agent admits failure
  instead of returning a confident empty brief.
- `ProgressFunc` (`:123`) reports phase transitions so a caller can render a
  live trace — already used by the mail service and available to chat.
- Verification is a distinct pass. Findings carry the source they were checked
  against, which is what makes the Mail Agent's "never invent citations" rule
  enforceable.

The test suite is substantial and green.

**The brief's confidence is justified.** The logic is the strongest part of the
codebase.

---

## The real gap: research runs leave no trace

There is no `research_runs` table. `grep -rn "research_runs" server/migrations/`
returns nothing. There is no repository, no run row, no persistence of any kind.
`research.Agent.Research(...)` is a synchronous function that returns a `Brief`
in memory and forgets it.

Four consequences, each of which is visible to a user:

**1. The Research Agent can never appear on the agent board.** The board unions
`multi_agent_jobs`, `blog_runs` and `mail_threads`
(`internal/repository/multi_agent_repository.go:170-226`). Research writes to
none of them. It is structurally impossible for it to show as busy, no matter
what Plan 4 does.

**2. `mail_threads.research_brief_id` points at nothing.** Migration
`000033_mail_agent.up.sql:82` and `:106` declare `research_brief_id UUID` — with
no foreign key, because there is no table to reference. `mail_service.go:507`
faithfully stores an ID that can never be dereferenced. The brief text survives
only because `ResearchSummary` and `ResearchFindings` are copied alongside it.

**3. The Task Manager works around it in the browser.** `launch.ts:88`:

```ts
case "researcher": {
  const { data: brief } = await apiClient.post<ResearchBrief>("/research", {...},
    { timeout: 180_000 });                       // sync call, up to 3 minutes
  const updated = await updateTask(task.id, {
    description: formatResearchBrief(brief, task.description),
    status: "done",
  }).catch(() => task);
```

The findings are flattened into the task's **description field** as markdown —
because there is nowhere else to put them. If the browser tab closes during
those three minutes, the run is lost with no record it happened.

**4. Chat research evaporates.** `specialists.go:38` `research_run` calls
`d.Research.Research(...)` and returns the brief inline. Ask a follow-up in the
next turn and the findings are gone unless they survived the chat summary.

A 180-second synchronous HTTP call held open by a browser is the load-bearing
design here. That is the thing to fix.

---

## Evaluations

`server/eval/research/`. Fake HTTP for all backends; fixture pages on disk.

### Suite A — brief quality

| # | Case | Assert |
|---|---|---|
| 1 | topic with good sources | `IsUsable` true; findings ≥ `MinFindings`; every finding has a source URL |
| 2 | every fetch 404s | `IsUsable` **false**; no fabricated findings; error surfaced |
| 3 | source contradicts the claim | claim dropped or flagged by `verify` |
| 4 | one backend times out | brief still returned from the rest; warning recorded |
| 5 | empty topic | rejected before any fetch |
| 6 | phases | `ProgressFunc` receives Planning, Searching, Reading, Verifying in order |

Case 2 is the important one. A research agent that returns a confident brief
when it read nothing is worse than one that errors, and `IsUsable` exists
precisely to prevent it — so it needs a test that would notice if it broke.

### Suite B — source backends

One case per backend (`arxiv`, `github`, `hackernews`, `feeds`, `jina`) against
recorded fixture responses: parses correctly, handles an empty result set, and
handles a malformed payload without panicking.

### Suite C — consumption by the other agents

| # | Case | Assert |
|---|---|---|
| 1 | Article Agent research stage | unusable brief does not silently produce an article of invention |
| 2 | Mail Agent handoff | brief reaches `BuildDraftPrompt`; draft URLs ⊆ brief source URLs |

Suite C is the cross-agent contract, and it is where a regression would do the
most damage.

### Tier 2

Five real topics against the live internet. Report to `eval/out/research.md`
with per-topic usability, source count, and wall time. Judges recency and
relevance, which fixtures cannot.

---

## Implementation

### Phase 1 — persist runs (the actual fix)

Migration `0000NN_research_runs.up.sql`:

```sql
CREATE TABLE research_runs (
    id              UUID PRIMARY KEY,
    org_id          UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    task_id         UUID REFERENCES tasks(id) ON DELETE SET NULL,
    requested_by    UUID REFERENCES users(id) ON DELETE SET NULL,
    source          TEXT NOT NULL,          -- 'task_manager' | 'chat' | 'mail' | 'blog'
    topic           TEXT NOT NULL,
    context         TEXT,
    status          TEXT NOT NULL,          -- queued|planning|searching|reading|verifying|completed|failed
    phase           TEXT,                   -- live ProgressFunc phase, for the board
    brief           JSONB,                  -- the whole research.Brief
    usable          BOOLEAN,
    error_message   TEXT,
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX research_runs_org_created ON research_runs (org_id, created_at DESC);
```

Store the brief as `JSONB` rather than normalised finding/source tables. The
brief is read whole, always, by every consumer; normalising it buys nothing and
costs a join plus two more migrations. Revisit only if findings ever need to be
queried independently.

Then `internal/repository/research_repository.go`, and a thin service that wraps
the existing `research.Agent` with run rows and a `ProgressFunc` that writes
`phase`. **Do not change `internal/research/`.** The package is correct; it
should stay a pure library with persistence layered above it.

Backfill the dangling reference: add the FK from `mail_threads.research_brief_id`
to `research_runs(id)` once rows exist, or rename the column to
`research_run_id` for honesty. Prefer the rename — the current name describes a
table that will not exist.

### Phase 2 — async run API

```
POST   /api/v1/research           → 202 {run_id}   (async; keep sync behind ?wait=true)
GET    /api/v1/research/runs/{id} → status, phase, brief
GET    /api/v1/research/runs      → list for the org
```

Keep `?wait=true` so `launch.ts` and the mail service keep working while they are
migrated. Then change `launch.ts:88` to start the run and poll — deleting the
180-second browser timeout and the description-field workaround.

### Phase 3 — board visibility

Add a `research_activity` arm to the union in
`multi_agent_repository.go:170-226`, mapping `status`/`phase` onto board
activities: `planning|searching|reading|verifying → executing`, `completed →
idle`, `failed → failed`.

This is the change that makes the Research Agent visible on the board for the
first time, and it is impossible without Phase 1.

### Phase 4 — surface warnings

When `IsUsable` is false, say so in the UI rather than presenting a thin brief as
a result. The agent already knows; the interface throws the information away.

---

## Acceptance criteria

- [ ] `research_runs` migrates up and down cleanly
- [ ] Every research entry point — Task Manager, chat, mail, blog — creates a run row
- [ ] Research Agent appears on the agent board while running
- [ ] `launch.ts` no longer holds a 180s request or writes findings into the task description
- [ ] `mail_threads` research reference resolves to a real row
- [ ] Suites A, B, C green; case 2 (`IsUsable` false on total fetch failure) Fatal
- [ ] `internal/research/` is unchanged apart from additive hooks
