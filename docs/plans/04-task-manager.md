# Plan 4 — Task Manager

> ## Execution status — 2026-08-29 — **partially done**
>
> **Done — defect 1, the live drift, is fixed and cannot silently recur:**
>
> - `agentschema` gained the Mail playbook (six fields; it had none), so the
>   Mail Agent is now driveable from chat as well as the Task Manager.
> - `GET /api/v1/agent-schemas` exposes the Go contract.
> - `TestGoAndTypeScriptSchemasAgree` parses `input-schemas.ts` and compares
>   keys, order and required-ness. It runs in the existing `go test ./...` — the
>   web app has only Playwright, and adding a JS unit runner for one test was
>   not worth it.
> - **The parity test immediately found drift this plan had missed:** the
>   pentester's `max_budget` and `instruction` were in opposite orders on the
>   two sides, and `scan_mode` was required in TypeScript but not in Go. Both
>   aligned.
> - Mail's chat routing no longer keys off `strings.Contains(prompt, "sync")`;
>   it goes through the normal schema path, and `mail_sync` now accepts and
>   saves the playbook (omitted fields never wipe a saved one).
>
> **Corrected: this plan's `dry_run` recommendation was wrong.** It advised
> defaulting both sides to preview-only. Migration `031_pr_reviewer_post_by_default`
> is an explicit product decision that the reviewer *posts* by default, and it
> reseeded the agent's system prompt to say so. Meanwhile `review.go` defaults
> `dry := true`. So chat is consistently preview-only, the Task Manager posts,
> and the system prompt claims posting. Reconciling changes whether an agent
> writes on a public pull request, which is a product call, not a tidy-up — so
> the divergence is now **pinned by `TestKnownDefaultDivergence_PRReviewerDryRun`**
> and documented at the field, rather than guessed at. **This needs your call.**
>
> **Not done — the Agent Run Contract (phases 1–4).** `POST /api/v1/agent-runs`,
> the `agent_runs` table, collapsing `launch.ts`'s client-side switch, unifying
> the board on one join, and routing chat through it. This is the large,
> invasive part, and doing it hurriedly would be worse than not doing it. Note
> that defect 3 is now materially smaller: research runs reach the board (plan
> 3), leaving pentest, review and generic `task_runs` outside it.

Verified against `feat/landing-page` @ `063cce3`. This is the spine: Plan 5
depends on it, Plan 2 Phase 4 depends on it, and Plan 3 Phase 3 feeds it.

The pre-merge version of this document said the Task Manager could not run
builtin agents and had no typed inputs. Both were fixed in
`feat/task-manager-agent-runs` / `feat/mail-research-links`. The problem now is
different and more subtle: **it works three separate times.**

---

## What is actually in the repo

Typed inputs exist. `web/nextjs/lib/agents/input-schemas.ts` (440 lines) defines
per-agent field sets with labels, placeholders, help text, validation and
`titleFrom` / `descriptionFrom` for the board card:

| Agent | Fields |
|---|---|
| `article_writer` | topic*, context, model |
| `researcher` | topic*, context |
| `pentester` | target*, scan_mode (quick/standard/deep), instruction, max_budget |
| `pr_reviewer` | repo*, pr_number*, dry_run |
| `mail` | senders, subject_prefixes, labels, knowledge URLs, research focus, reply style |
| *(default)* | prompt* |

`launch.ts:39` `launchAgentForTask` creates the board task first, then dispatches
per agent. The mail case even handles the "empty form must not wipe a saved
playbook" edge (`mailFormIsBlank`) and treats 409/503 as "playbook saved, Gmail
not connected" rather than an error. That is careful work.

So the brief's "some agents get the right input prompts, others don't" is no
longer literally true from the Task Manager. It **is** true across surfaces, and
that is the defect.

---

## Three defects, stated precisely

### Defect 1 — the input contract exists twice, by hand, and has already drifted

`internal/agentschema/schema.go:1`:

> *"Package agentschema is the server copy of Task Manager's agent input contract
> (web/nextjs/lib/agents/input-schemas.ts). Keep the required field keys and
> order in sync with that file."*

A comment asking a human to keep two files in sync is a defect with a deadline.
The deadline has passed:

| Agent | `input-schemas.ts` | `agentschema/schema.go` |
|---|---|---|
| article_writer | topic, context, model | topic, context, model ✅ |
| researcher | topic, context | topic, context ✅ |
| pentester | target, scan_mode, instruction, max_budget | same ✅ |
| pr_reviewer | repo, pr_number, dry_run **default `false`** | repo, pr_number, dry_run **default `"true"`** ❌ opposite defaults |
| **mail** | **six fields** | **zero fields** ❌ |

The Mail row is why the Mail Agent is good from the Task Manager and nearly
unusable from chat (Plan 1, Gap A).

The `pr_reviewer` row is worse than a missing field, because the two copies
disagree about a **side effect**:

- `agentschema/schema.go:76` — `{Key: "dry_run", Label: "Preview only", Default: "true"}`
- `input-schemas.ts:214-217` — `{key: "dry_run", type: "checkbox", defaultValue: false}`

Ask chat to review a PR and it defaults to preview-only. Submit the Task Manager
form without ticking the box — the default — and it **posts review comments to a
public GitHub pull request**. Same agent, same field, opposite defaults, and the
difference is whether strangers see the output. Neither default is wrong on its
own; having both is.

### Defect 2 — dispatch happens in the browser, three ways

`launch.ts` is a client-side switch that posts to a different endpoint per
agent — `/pentest-runs`, `/review-runs`, blog generate, `/research`,
`/mail/connection` + `/mail/sync`. Only the `default` branch calls
`createTaskRun`.

That means:

- **The server has no single entry point for "run agent X".** Any non-browser
  caller — chat, Telegram, a scheduler, an API client — must re-implement the
  fan-out. Chat already has: `internal/platformtools/execute.go`.
- **A closed tab is a lost run.** The researcher case holds a 180-second request
  and then writes results into the task description (Plan 3).
- **Partial failure is unhandled.** The task row is created before dispatch; if
  dispatch throws, a board task exists for a run that never started.

`TaskRunService.CreateRun` (`internal/service/task_run_service.go:63`) — the one
thing that looks like a server-side front door — has **no builtin awareness at
all**. `grep -n "Builtin" internal/service/task_run_service.go` returns nothing.
It builds a prose prompt from the task title and description
(`buildRunPrompt:230`) and runs a generic LLM loop. Route the Article Writer
through it and you get a paragraph about writing an article, not an article.

### Defect 3 — four of six run types never reach the board

Board sources, `internal/repository/multi_agent_repository.go:170-226`:
`multi_agent_jobs`, `blog_runs`, `mail_threads` (twice). Therefore:

| Run type | Table | On board |
|---|---|---|
| Article | `blog_runs` | ✅ |
| Mail | `mail_threads` | ✅ |
| Pentest | `pentest_runs` | ❌ |
| PR review | `review_runs` | ❌ |
| Research | *nothing* | ❌ |
| Generic | `task_runs` | ❌ |

The brief asks that "a person can assign a task through an agent and see it
reflected on the agent/task board." For four of six paths, they cannot.

---

## The fix: one Agent Run Contract

One server-side front door that every surface calls:

```
POST /api/v1/agent-runs
{ "agent_id": "...", "task_id": "...", "inputs": { "topic": "..." } }
→ 202 { "run_id": "...", "kind": "article_writer", "status": "queued" }
```

The server owns validation, dispatch, the run row and the board entry. Callers
— Task Manager, chat, Telegram, scheduler — supply `agent_id` and `inputs` and
nothing else.

### Shape

```go
// AgentRunner executes one builtin. Registered by builtin marker, so adding an
// agent is a registration rather than an edit to a switch that three surfaces
// have to learn about.
type AgentRunner interface {
	Builtin() string
	Schema() agentschema.Schema
	Start(ctx context.Context, run *model.AgentRun) error
}

type AgentRunService struct {
	runners map[string]AgentRunner   // builtin marker → runner
	generic AgentRunner              // prompt-driven fallback
}
```

`Start` is called on a worker, not in the request. The HTTP handler validates
against `Schema()`, writes the run row, returns 202.

### Why a registry rather than a switch in `TaskRunService`

A switch would work and would be a smaller diff. The registry earns its keep for
one reason: **the schema and the executor stop being able to disagree.** Today
they live in different languages in different directories, which is exactly how
`mail` ended up with six fields on one side and zero on the other. Making
`Schema()` a method on the thing that runs makes the drift unrepresentable.

### Validation lives in one place

`agentschema.Schema` already has `NextMissing` and `ApplyDefaults`
(`schema.go:118`, `:140`) and they are tested. Reuse them verbatim — the missing
piece is not logic, it is a caller. `NextMissing` returning a slot becomes
`400 {"missing": ["topic"], "question": "What should I write about?"}`, which is
the same shape chat already renders as a clarifying question.

---

## Phase 1 — the contract

1. `internal/model/agent_run.go` — `AgentRun{ID, OrgID, TaskID, AgentID, Builtin,
   Inputs JSONB, Status, Kind, ExternalRunID, Error, timestamps}`.
2. Migration `agent_runs`, with `external_run_id` pointing at the specialist row
   (`blog_runs.id`, `pentest_runs.id`, `research_runs.id`, …) so one table can
   answer "what is running" without knowing every specialist's storage.
3. `AgentRunner` implementations wrapping the **existing** services: article
   (`blog.Runner.Generate`), research (Plan 3's async service), pentest, review,
   mail, generic (today's `TaskRunService` path). No new execution logic — this
   is a re-plumbing, not a rewrite.
4. `POST /api/v1/agent-runs`, `GET /api/v1/agent-runs/{id}`, `GET /api/v1/agent-runs`.

### Fix the two-copies problem

Two options. **Take the second one first** — it is a fraction of the work and
removes the actual pain:

**Option A — generate the TypeScript.** `go run ./cmd/gen-schemas` emits
`input-schemas.generated.ts` from `agentschema`; CI fails if the file is dirty.
Single source of truth, but adds a build step and a code generator to maintain.

**Option B — contract test.** Expose `GET /api/v1/agent-schemas` returning the Go
schemas, and add a Vitest case asserting `input-schemas.ts` matches it key-for-key
and order-for-order. Drift becomes a red test on the next commit instead of a
bug found in production. ~40 lines, no build step.

Either way, first: **give `agentschema` the Mail fields it is
missing and reconcile the `dry_run` default.** That closes the live drift
regardless of which option is chosen, and unblocks Plan 1 Gap A.

On `dry_run`, pick the safe default deliberately rather than by accident:
default to preview-only (`true`) on both sides, so posting to a public PR is
something a person opts into. That is a behaviour change to the Task Manager
form and should be called out in the PR description, not slipped in.

## Phase 2 — the dialog asks the right questions

The Task Manager form is already good. The change is that it stops dispatching:

- On agent select, fetch the schema (or use the shared module) and render fields.
- On submit, `POST /api/v1/agent-runs` — **one call**. Delete the per-agent
  branches from `launch.ts`; keep `mail-playbook.ts`'s "don't wipe a saved
  playbook" logic by moving it into the mail runner where it belongs on the server.
- Render `400 {missing, question}` inline against the offending field.
- Task creation and run creation become one server-side transaction, closing
  Defect 2's orphaned-task case.

`launch.ts` shrinks from a 153-line switch to a thin call. That deletion is the
main deliverable of this phase.

## Phase 3 — everything lands on the board

With `agent_runs` as the common table, the board query stops being a growing
union of specialist tables and becomes one join:

```sql
SELECT a.id, a.name, a.role, r.status, r.phase, r.created_at
FROM agents a
LEFT JOIN LATERAL (
    SELECT status, phase, created_at FROM agent_runs
    WHERE agent_id = a.id AND org_id = a.org_id
    ORDER BY created_at DESC LIMIT 1
) r ON TRUE
WHERE a.org_id = $1
```

Keep the existing `multi_agent_jobs` arm — collaboration jobs are a genuinely
different thing. Retire the `blog_runs` and `mail_threads` arms once those
runners write `agent_runs` rows, and delete them in a follow-up rather than the
same PR, so a bad cutover is one revert.

Map `agent_runs.status` onto the existing `Activity*` constants; do not invent
new board columns. `web/nextjs/lib/api/agent-board.ts:21` already warns that the
board silently drops unknown activities — an agent would vanish rather than error.

## Phase 4 — one front door

Point chat at it. `internal/platformtools/execute.go:133` `dispatchTool` calls
the specialist directly; replace with a call to `AgentRunService.Start`. Chat
then reports "started, here is the run" and the run appears on the board like any
other. That is Plan 5 Phase 3, and it is a handful of lines **once this exists**.

Retire the direct specialist routes (`/blogs/generate`, `/research`,
`/pentest-runs`, `/review-runs`) from the UI. Keep them on the server for now;
they are useful for debugging and for Tier 2 evals.

---

## Evaluations

`server/eval/taskmanager/`, Tier 1, fake runners recording what they receive.

| # | Case | Assert |
|---|---|---|
| 1 | each builtin, valid inputs | correct runner invoked; run row `queued`; 202 |
| 2 | article run, no topic | 400 with `missing:["topic"]` and the schema's question |
| 3 | pentest run, no scan_mode | defaults to `quick` (`ApplyDefaults`) |
| 4 | unknown agent id | 404, no run row |
| 5 | runner returns error | run row `failed` with message; task not orphaned |
| 6 | every builtin | appears on the board within one poll |
| 7 | **schema parity** | Go schema == TS schema: keys, order **and defaults** |
| 8 | chat and Task Manager, same inputs | identical run rows |

Case 7 is the regression net for Defect 1 — the one that would have caught both
the Mail drift and the `dry_run` default conflict. Assert defaults, not just
field names: the `dry_run` divergence is invisible to a keys-only comparison.

Case 8 is the regression net for Defect 2: it is the executable form
of "there is one way to run an agent."

---

## Acceptance criteria

- [ ] `POST /api/v1/agent-runs` is the only path any UI uses to start an agent
- [ ] `launch.ts` no longer contains a per-agent dispatch switch
- [ ] `agentschema` has Mail fields; `dry_run` defaults agree and default to preview-only; parity test green
- [ ] All six run types appear on the agent board
- [ ] Missing required input → 400 with the field and question, rendered inline
- [ ] Task and run are created together; no orphaned board tasks on dispatch failure
- [ ] Chat-initiated and Task-Manager-initiated runs are indistinguishable in `agent_runs`
- [ ] `go test ./...` green; eval suite green

## Risk note

This is the largest change in the set and it touches the surface a user sees
every day. Sequence it so each phase ships independently: Phase 1 adds an unused
API, Phase 2 switches one caller, Phase 3 changes one query, Phase 4 switches
the second caller. If Phase 3 misbehaves the board is one revert away, and no
run data is lost because `agent_runs` is additive to the specialist tables
rather than a replacement for them.
