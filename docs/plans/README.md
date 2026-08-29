# JobShout agent programme — plan index

> ## Execution status — 2026-08-29
>
> Plans 1, 2 and 3 are done. Plan 4 is partially done (the live schema drift is
> fixed and guarded; the Agent Run Contract is not built). Plan 5's memory fix
> is done; its eval suites and the routing phase are not.
>
> Each plan carries its own status block. Two things worth reading there:
> plan 1's eval found the exact bug it was written for, and plan 4's `dry_run`
> recommendation turned out to be **wrong** — it needs a product decision from
> you, and is pinned by a test until you make it.
>
> Everything below `go test ./...` green: 30 packages, including three new
> suites (`eval/harness`, `eval/mail`, `eval/research`).

Rewritten 2026-08-29 against `feat/landing-page` @ `063cce3`, **after** the
master merge that brought in `feat/mail-research-links` and
`feat/chat-agent-reliability`. Every claim below was re-checked against code at
this commit; `file:line` references are load-bearing.

Baseline: `cd server && go test ./...` is **green** (27 packages `ok`, 0 `FAIL`).

> **These documents replace the set written earlier today at `d9f8099`.** That
> set was written before the merge and its two headline findings are now wrong:
> it said the Gmail Agent did not exist, and that the Task Manager had no typed
> inputs. Both were built in the merged branches. See "What changed" below.

| # | Plan | Shape of the work |
|---|------|-------------------|
| 0 | this file | cross-cutting findings, shared eval harness, sequencing |
| 1 | [Mail (Gmail) Agent](./01-gmail-agent.md) | **evals only** — the agent is built and does the brief's use case |
| 2 | [Article Agent](./02-article-agent.md) | one prompt fix + cover diversity + evals |
| 3 | [Research Agent](./03-research-agent.md) | evals + persistence — logic is solid, runs vanish |
| 4 | [Task Manager](./04-task-manager.md) | collapse three parallel dispatchers into one contract |
| 5 | [Chatbot](./05-chatbot.md) | memory repair, routing through #4, model A/B |

---

## What changed since the pre-merge plan set

| Pre-merge claim | Status now | Evidence |
|---|---|---|
| "There is no Gmail Agent" | **Wrong.** Fully built. | `server/internal/mail/` (1,504 lines), `internal/service/mail_service.go`, migration `000033_mail_agent.up.sql` |
| "Task Manager has no typed inputs" | **Wrong.** Typed per-agent forms exist. | `web/nextjs/lib/agents/input-schemas.ts` (440 lines) |
| "Builtin marker never consulted on any execution path" | **Wrong.** Consulted in two places. | `web/nextjs/lib/agents/launch.ts:47`, `internal/platformtools/execute.go:27` |
| "Chat cannot ask for missing inputs" | **Wrong.** It interviews. | `internal/agentschema/schema.go:118` `NextMissing` |
| "Research runs are not persisted" | **Still true.** | no `research_runs` table in `server/migrations/` |
| "Chat execution bypasses the board" | **Still true.** | `internal/platformtools/execute.go:133` `dispatchTool` |
| "No eval harness anywhere" | **Still true.** | only stale JSON in `docs/chat-agent-eval-prompt.md` |

The work got smaller and sharper. It is no longer "build a Gmail agent"; it is
"prove the agents work, and unify how they are launched."

---

## Four findings that shape the work

### 1. The Mail Agent already does the brief's headline use case

The brief's example — a client asks about a machine and references a link; the
agent researches the link, then drafts a reply — is implemented:

`internal/service/mail_service.go:463` `processThread` classifies the message
(`class.NeedsResearch`), and at `:479`:

```go
wantResearch := pinned || class.NeedsResearch
if wantResearch && s.research != nil && s.research.Available() {
    th.Status = model.MailThreadResearching
    b, rerr := s.research.Research(ctx, th.OrgID, req, nil)
    ...
    th.ResearchSummary, th.ResearchBriefID, th.ResearchFindings = ...
}
```

The brief is then folded into the draft prompt (`internal/mail/draft.go:102`
`BuildDraftPrompt`). Drafts are never sent without human approval
(`POST /mail/drafts/{id}/approve`), and `HeuristicDraft` is unit-tested to never
claim a message was sent.

**Consequence:** Plan 1 is an evaluation plan, not a build plan. It is now the
*smallest* of the five, not the largest.

### 2. There are three parallel implementations of "run agent X with inputs Y"

| Dispatcher | Language | Where | Reaches |
|---|---|---|---|
| `launchAgentForTask` | TypeScript, **runs in the browser** | `web/nextjs/lib/agents/launch.ts:39` | `/pentest-runs`, `/review-runs`, blog generate, `/research`, `/mail/*` |
| `runAgentExecute` | Go, server | `internal/platformtools/execute.go:13` | platform tool registry |
| `TaskRunService.CreateRun` | Go, server | `internal/service/task_run_service.go:63` | generic LLM loop only |

They share no code. The input contract is duplicated by hand across
`input-schemas.ts` and `internal/agentschema/schema.go` — and the file header of
the latter admits it:

> *"Package agentschema is the server copy of Task Manager's agent input
> contract (web/nextjs/lib/agents/input-schemas.ts). Keep the required field
> keys and order in sync with that file."*

**They have already drifted.** The Mail agent has a full field set in TypeScript
(senders, subject prefixes, labels, knowledge URLs, research focus, reply style)
and **zero fields** in Go (`schema.go:79-83`). This is why Mail behaves well from
the Task Manager and poorly from chat.

This is the root cause behind the brief's "some agents get the right input
prompts, others don't." It is Plan 4.

### 3. The agent board is fed by three tables; most run types are not among them

`internal/repository/multi_agent_repository.go:170-226` unions exactly:

```
job_activity            (multi_agent_jobs)
blog_activity           (blog_runs)
mail_activity           (mail_threads)
mail_research_activity  (mail_threads)
```

| Agent | Writes to | On the board? |
|---|---|---|
| Article Writer | `blog_runs` | ✅ |
| Mail | `mail_threads` | ✅ |
| Researcher | *nothing* | ❌ |
| Pentester | `pentest_runs` | ❌ |
| PR Reviewer | `review_runs` | ❌ |
| Generic agent | `task_runs` | ❌ |

So "assign a task and see it on the board" works for two of six paths. Plan 4
fixes the wiring; Plan 3 gives Research something to write.

### 4. Chat-triggered runs still bypass the Task Manager

`internal/platformtools/execute.go:133`:

```go
func dispatchTool(ctx context.Context, reg *Registry, name string, input map[string]any) (*Result, error) {
	t, ok := reg.Get(name)
	...
	return t.Run(ctx, input)
}
```

The specialist tool is invoked directly. No task row, no `task_runs` row, no
board entry. `research_run` calls `d.Research.Research(...)` inline
(`specialists.go:38`) and the findings exist only inside that one chat reply.

This is precisely the user's stated requirement — *"if an agent is executed
through the chatbot, that execution should go through the Task Manager"* — and
it is still unmet. Plan 5 Phase 3, which depends on Plan 4.

---

## Shared foundation: the eval harness

Nothing here exists yet. Both Plan 1 and Plan 3 are blocked on it, so build it
first. It is roughly a day of work and everything else leans on it.

### Two tiers, deliberately

**Tier 1 — hermetic, in CI, deterministic.** Fake LLM, fake HTTP, fake image
service, fake Gmail. Runs on every push, no network, no GPU, no API keys. These
are the gate: a red Tier 1 blocks a merge.

**Tier 2 — live, manual, behind a build tag.** Real Ollama, real fetches, real
Gmail. Answers "is the *output* any good", which Tier 1 structurally cannot.
Never in CI: non-deterministic and needs the workstation.

```
go test ./...                      # Tier 1
go test -tags=evallive ./eval/...  # Tier 2
```

### Layout

```
server/eval/
  harness/          # shared: fake LLM, fixture loading, scoring, report writer
    fakellm.go      # scripted responses keyed by prompt substring
    score.go        # Check{Name, Fatal, Fn}; Report{Passed, Failed, Notes}
    report.go       # writes eval/out/<suite>.json + .md
  mail/             # Plan 1
    fixtures/       # *.json inbox messages + expectations
  article/          # Plan 2
  research/         # Plan 3
  taskmanager/      # Plan 4
  chat/             # Plan 5
    fixtures/       # transcripts: user turns + expected tool calls
```

### The one rule that makes these evals worth writing

**Assert on structure and behaviour, not on prose.** "Did the draft cite the URL
the sender pasted?" is a check. "Is the draft well written?" is a rubric, and
rubrics go in Tier 2 where a human reads the report.

A `FakeLLM` returning canned strings makes the *pipeline* testable — which
branch ran, which tool was called, what got persisted, what the caller saw.
That is where the current bugs actually live.

---

## Sequencing

Plan 4 is the spine. Two things depend on it and it depends on nothing.

```
        ┌─────────────────────────┐
Step 0  │ eval harness (½–1 day)  │
        └────────────┬────────────┘
                     │
     ┌───────────────┼────────────────┬─────────────────┐
     ▼               ▼                ▼                 │
┌─────────┐   ┌─────────────┐  ┌──────────────┐         │
│ Plan 1  │   │  Plan 2     │  │  Plan 3      │         │
│ Mail    │   │  Article    │  │  Research    │         │
│ evals   │   │  prompt fix │  │  persistence │         │
└─────────┘   └─────────────┘  └──────┬───────┘         │
                                      │                 │
                                      ▼                 ▼
                              ┌───────────────────────────┐
                              │ Plan 4 — Task Manager     │
                              │ one dispatcher, one board │
                              └────────────┬──────────────┘
                                           ▼
                              ┌───────────────────────────┐
                              │ Plan 5 — Chatbot          │
                              │ memory, routing via #4    │
                              └───────────────────────────┘
```

**Recommended order:** harness → Plan 1 (cheapest, highest confidence gain) →
Plan 2 upgrade 1 (a one-line prompt fix with visible payoff) → Plan 3 Phase 1
(gives Research a run row, which Plan 4 needs) → Plan 4 → Plan 5.

### Rough sizing

| Plan | Tier 1 evals | Code change | Notes |
|---|---|---|---|
| 0 harness | — | ½–1 day | blocks 1 and 3 |
| 1 Mail | 10 cases | ~none | agent is built; this is verification |
| 2 Article | 6 cases | small + medium | prompt fix is one paragraph |
| 3 Research | 10 cases | medium | migration + repo + board wiring |
| 4 Task Manager | 8 cases | **large** | the spine; touches Go and TS |
| 5 Chatbot | 12 cases | medium | memory rewrite + routing |

### One judgement call worth surfacing

Plan 4 proposes generating `input-schemas.ts` from the Go `agentschema` package
rather than keeping two hand-maintained copies. That is a build-step change and
therefore the most invasive suggestion in this set. The alternative — a contract
test that fails when the two drift — is cheaper and gets most of the benefit.
Plan 4 specifies both; take the contract test first if you want the smaller
diff.
