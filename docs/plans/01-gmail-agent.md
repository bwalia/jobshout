# Plan 1 — Mail (Gmail) Agent

> ## Execution status — 2026-08-29
>
> **Done.** Shared eval harness built (`server/eval/harness/`), ten fixtures and
> the full check set at `server/eval/mail/` (86 checks, green), Tier 2 skeleton
> behind `-tags=evallive`.
>
> **The suite found the bug it was written to find.** Three fixtures failed on
> first run — `price_with_link`, `availability_with_link`, `multi_link`, i.e.
> exactly the brief's headline use case. A link the *sender* pasted was never
> put in `research.Request.URLs`, so it was searched around rather than read;
> only operator-pinned URLs took the direct-fetch path. Fixed with
> `mail.SenderLinks` (`internal/mail/links.go`, with tests) wired into
> `processThread`. Footer/tracking/social/image links are filtered so a
> signature cannot become the only source a reply may cite.
>
> Also closed: the thread's research reference used to be a fresh `uuid.New()`
> naming nothing; it now stores the real `research_runs.id` (see plan 3), with a
> Fatal check asserting it.
>
> **Gap A is closed** — see plan 4. `agentschema` has the Mail playbook, the
> `strings.Contains(prompt, "sync")` router is gone, and `mail_sync` accepts and
> saves the playbook.

**Status: built and shipped. This plan is evaluations, plus two small gaps.**

Verified against `feat/landing-page` @ `063cce3`. The pre-merge version of this
document said the agent did not exist; that was true at `d9f8099` and is wrong
now. `feat/mail-research-links` built it.

---

## What is actually in the repo

`server/internal/mail/` — 1,504 lines, with tests:

| File | Lines | What it does |
|---|---|---|
| `gmail.go` | 390 | OAuth exchange/refresh, `Profile`, `ListMessages`, `Send`, MIME part walking |
| `draft.go` | 184 | `BuildDraftPrompt`, `HeuristicDraft`, reply-subject handling |
| `classify.go` | 165 | LLM classifier + `HeuristicClassify` fallback |
| `oauth.go` | 91 | authorise URL, state |
| `types.go` | 86 | `InboxMessage`, `OutboundMessage`, `ClassifyResult`, `Draft` |
| `crypto.go` | 72 | AES token encryption at rest |
| `knowledge.go` | 53 | pinned knowledge URLs |
| `config.go` | 59 | `MAIL_*` env, `Configured()` |
| `redact.go` | 27 | strips tokens from error strings before they reach a user |

Service: `internal/service/mail_service.go` (`EnsureMailAgent`, `EnqueueSync`,
`SyncNow`, `ProcessDueSyncs`, `syncConnection`, `ingestAndProcess`,
`processThread`, `ApproveSend`), plus `mail_reconciler.go`.

Storage: migration `000033_mail_agent.up.sql` — `mail_connections`,
`mail_threads`, `mail_drafts`.

Routes (`internal/handler/mail_handler.go`):

```
GET    /api/v1/mail/connection
PATCH  /api/v1/mail/connection
DELETE /api/v1/mail/connection
POST   /api/v1/mail/connection/oauth/start
GET    /api/v1/mail/connection/oauth/callback
POST   /api/v1/mail/sync
GET    /api/v1/mail/threads
GET    /api/v1/mail/threads/{id}
GET    /api/v1/mail/drafts
PATCH  /api/v1/mail/drafts/{id}
POST   /api/v1/mail/drafts/{id}/approve
POST   /api/v1/mail/drafts/{id}/reject
```

Builtin marker `model.BuiltinMail` (`internal/model/agent.go:60`), seeded by
`mailAgentSeed` with the system prompt:

> *"You are the Mail Agent. You triage the organisation inbox, draft replies, and
> never send until a human approves. … Work that needs facts is handed to the
> Research Agent — you do not invent citations."*

---

## The brief's use case is implemented

> *A client emails asking "What is the price of this machine?" and references a
> link. The agent should research the link, then draft a reply from it.*

`mail_service.go:463` `processThread`:

1. `s.classifier.Classify(...)` → `ClassifyResult{NeedsResearch, ...}`
2. `:479` `wantResearch := pinned || class.NeedsResearch` — `pinned` covers the
   operator's saved knowledge URLs, `NeedsResearch` covers the sender's link
3. `:483` thread moves to `model.MailThreadResearching` (a board-visible state)
4. `:496` `s.research.Research(ctx, th.OrgID, req, nil)`
5. `:505-509` brief stored as `ResearchSummary`, `ResearchBriefID`,
   `ResearchFindings`
6. brief passed into `BuildDraftPrompt` (`draft.go:102`)
7. draft persisted; **nothing is sent** until `ApproveSend`

Degradation is handled: `:498` logs and drafts without research if the handoff
fails; `:512` logs if research was wanted but unavailable. Neither aborts the
reply — correct behaviour.

So the remaining question is not *"does it work?"* but *"can we prove it works,
and keep it working?"* That is what this plan builds.

---

## Two real gaps

### Gap A — the Mail Agent is nearly unusable from chat

`internal/agentschema/schema.go:79`:

```go
case model.BuiltinMail:
	return Schema{
		Builtin:        model.BuiltinMail,
		SpecialistTool: "mail_list_drafts",
	}
```

**No fields at all** — while the Task Manager form for the same agent has six
(`input-schemas.ts` mail block: senders, subject prefixes, labels, knowledge
URLs, research focus, reply style).

And `execute.go:45`:

```go
if builtin == model.BuiltinMail {
	tool := "mail_list_drafts"
	if strings.Contains(strings.ToLower(prompt), "sync") {
		tool = "mail_sync"
	}
	return dispatchTool(ctx, reg, tool, input)
}
```

A substring match on the word "sync" is the entire routing logic. "check my
inbox" lists drafts; "sync my mail" syncs; "draft a reply to Dave about the
lathe price" lists drafts. There is no `mail_approve`, no `mail_draft_get`, no
way to set the playbook from chat.

This is a symptom of the two-copies problem in Plan 4, and the fix belongs
there. Recorded here because it is the Mail-specific face of it.

### Gap B — no evaluations

There is no test that runs a message through
classify → research → draft and asserts the outcome. The unit tests cover prompt
construction and heuristics in isolation, which is good but does not cover the
pipeline.

---

## Evaluations

Tier 1, hermetic. `server/eval/mail/`.

### Fakes needed

| Interface | Fake | Notes |
|---|---|---|
| `mail.GmailAPI` | `fakeGmail` | serves fixture `InboxMessage`s; records `Send` calls |
| `llm.Client` | `harness.FakeLLM` | scripted classify JSON and draft text |
| `ResearchService` | `fakeResearch` | returns a canned `research.Brief`; records the `Request` |

`GmailAPI` is already an interface (`gmail.go:22`) so no production change is
needed to fake it — a good sign about the design.

### Fixtures — `server/eval/mail/fixtures/*.json`

| # | Fixture | Scenario | Expected |
|---|---|---|---|
| 1 | `price_with_link` | "What's the price of this machine? https://example.com/lathe-9000" | `needs_research=true`; research called with the URL; draft cites it; not sent |
| 2 | `availability_with_link` | "Do you have the XR-200 in stock? <link>" | as above; draft references stock/availability |
| 3 | `question_no_link` | "What are your opening hours?" | `needs_research` may be false; draft produced; research **not** called with a fabricated URL |
| 4 | `newsletter` | marketing blast | classified ignore; **no draft row** |
| 5 | `auto_reply` | "Out of office" | ignored; no draft |
| 6 | `pinned_knowledge` | plain question, operator has pinned URLs | research called even though `needs_research=false` (`pinned` branch, `:479`) |
| 7 | `research_unavailable` | fixture 1, research service returns error | draft still produced; no invented citation; warning logged |
| 8 | `multi_link` | two URLs in the body | both reach the research request |
| 9 | `reply_style` | operator set reply style "terse, no greeting" | style string present in the draft prompt |
| 10 | `thread_reply` | message with `Re:` subject | `replySubject` does not double the prefix |

### Checks — deterministic, these are the gate

```go
Check{"never_sends_without_approval", Fatal: true,
    Fn: func(r Run) error { return requireZero(r.Gmail.SendCalls) }}

Check{"research_receives_sender_urls", Fatal: true,
    Fn: func(r Run) error { return requireContainsAll(r.Research.LastRequest, r.Fixture.URLs) }}

Check{"draft_cites_a_real_source", Fatal: true,
    Fn: func(r Run) error { return requireSubset(urlsIn(r.Draft.Body), r.Research.Brief.SourceURLs) }}

Check{"ignored_mail_produces_no_draft", Fatal: true, ...}
Check{"draft_never_claims_sent", Fatal: true, ...}   // regression net for the guard in draft.go
Check{"tokens_never_appear_in_errors", Fatal: true, ...}  // exercises redact.go
```

`draft_cites_a_real_source` is the important one: it is the fabrication guard.
A draft may only contain URLs the research brief actually returned. This is the
check that would catch the failure mode the system prompt warns about.

### Rubric — Tier 2, secondary

Real Ollama, real fetch, a handful of live messages. Scored 1–5 by an LLM judge,
written to `eval/out/mail.md` for a human to read:

- Does the reply answer the question that was asked?
- Is the tone consistent with the configured reply style?
- Would you send this without editing it?

Rubric scores never gate a merge. They tell you whether the model or the prompt
needs work, which the deterministic checks cannot.

---

## Implementation

There is very little. The agent is built; this is test scaffolding.

### Phase 1 — harness and fakes (½ day)

Build `server/eval/harness/` per the README, then `fakeGmail` and `fakeResearch`
in `server/eval/mail/`. No production code changes.

### Phase 2 — the ten fixtures and checks (½ day)

Write `eval/mail/fixtures/*.json` and `mail_eval_test.go`. Expect one or two
genuine bugs to surface here — the pipeline has never been run end-to-end under
assertion. Fix what surfaces; that is the point of the exercise.

### Phase 3 — Tier 2 live suite (¼ day)

`//go:build evallive`. Reads a real mailbox, writes `eval/out/mail.md`. Skips
cleanly when `MAIL_*` env is unset so it never fails a machine without Gmail.

### Phase 4 — close Gap A (deferred to Plan 4)

Give `agentschema` a real Mail schema and replace the `strings.Contains` router
with intent-shaped tools (`mail_sync`, `mail_list_drafts`, `mail_draft_get`,
`mail_approve`). Sequenced in Plan 4 because the fix is the shared contract, not
a Mail-specific patch.

---

## Acceptance criteria

- [ ] `go test ./eval/mail/...` green, ten fixtures, checks above
- [ ] `never_sends_without_approval` and `draft_cites_a_real_source` present and
      Fatal
- [ ] Fixtures 1 and 2 — the brief's literal use case — pass end to end
- [ ] Tier 2 suite runs on the workstation and writes a readable report
- [ ] Any bug found in Phase 2 is fixed, with the failing fixture kept as a
      regression test

## What this plan deliberately does not do

- Rebuild anything in `internal/mail/`. It works.
- Add IMAP or non-Gmail providers. Not asked for.
- Change the approve-before-send model. It is correct and should stay.
