# JobShout Chat Agent — Reliability, Correctness, and Model Routing

> Paste everything below the line into the implementing agent. Self-contained: a
> fresh session can execute this without other chat. Work on a new branch
> `feat/chat-agent-reliability`. Do not mix unrelated refactors into this work.
>
> Companion docs (context only — do not re-implement the old 8-intent router):
> `docs/chat-agent-upgrade-prompt.md`, `docs/chat-agent-fix-plan.md`.
> The tool-calling loop, ReAct fallback, SSE UI, and guard chain **already exist**.
> This prompt is the next slice: make execute interview for missing fields, stop
> guessing, and pin chat to a tool-capable model.

---

## ROLE

You are making the JobShout **chat agent** (the conversational control surface for
the job shop) **reliable and correct**. Product intent: a user can say “run the
research agent” with only a partial request, and chat **asks for the remaining
fields** before anything launches — the same fields Task Manager already requires.

Today chat is a Go tool-calling loop over platform tools. That shape stays. You
will not replace it with LangChain, a new intent classifier, or a second agent
framework.

Ship in the **phase order** below. Each phase is reviewable: tests pass,
`gofmt`/`go test` for touched packages, no drive-by refactors, no new markdown
unless a phase explicitly says to update this file’s checklist.

---

## PART 0 — GROUND TRUTH (verify, then implement)

Read these before writing code. Line numbers drift; search by symbol.

### What chat is

| Piece | Path |
|---|---|
| Tool-calling loop | `server/internal/chatagent/agent.go` |
| System prompt | `server/internal/chatagent/prompt.go` |
| ReAct fallback | `server/internal/chatagent/react.go` (`turnCallerFor`) |
| Window / summary / pending / entities | `server/internal/chatagent/memory.go` |
| Slot merge / confirm heuristics | `server/internal/chatagent/confirm.go` |
| Platform tools | `server/internal/platformtools/*.go` |
| Session persist | `server/internal/chatsvc/service.go` |
| Wiring | `server/cmd/server/main.go` — `chatagent.New(llmRouter.Default(), …)` |
| LLM clients | `server/internal/llm/{client,router,ollama,openai,claude}.go` |
| Task Manager field list (source of truth to port) | `web/nextjs/lib/agents/input-schemas.ts` |
| Task Manager launch | `web/nextjs/lib/agents/launch.ts` |
| Builtin markers | `server/internal/model/agent.go` (`BuiltinArticleWriter`, `researcher`, `pentester`, `pr_reviewer`, `mail`) |

Job-shop workers (Article Writer, Research Agent, …) are **not** the chatbot.
Chat starts them via tools. Do not change worker auto-select (`AUTO_MODEL_SELECTION`)
except where this prompt says chat gets its **own** client.

### How a turn works today

1. `chatsvc.SendTurn` persists the user message, loads last **80** rows, passes
   session metadata into `Agent.Run`.
2. Before the LLM: pending **confirmation** (approve/cancel) then pending
   **slot-fill**. Slot-fill **merges the user text into `args[missing[0]]` and
   re-runs the tool with no LLM** (`handlePending` + `mergePendingArgs`).
3. Else `loop`: system prompt + windowed history + tools → generate. Max **15**
   iterations. Guard: policy → RBAC → org scope → destructive confirm.
4. Tool result: `Missing`/`Question` → SSE `clarify` + `pending_action`;
   destructive → confirm card; else wrap as untrusted tool result and continue.
5. Persist **only** user text + sanitised assistant `message`. Tool-call /
   `RoleTool` turns are **not** stored. Continuity is metadata: `summary`,
   `entities` (`last_{kind}`, 24h), `pending_action`, `pending_confirmation`
   (10 min), `disclosed_tools`.
6. History window: ~**4000** estimated tokens (`len/4`), never fewer than **4**
   turns. Evicted user/agent lines roll into `summary` (cap ~3000 chars). No
   summarizer LLM. `remember` / `Recall` is separate long-term memory (user ID).

### Why execute is wrong today

**A. Generic execute has no specialist slots.** `agent_execute` schema is
`name?` + **required `prompt`**. Empty prompt → `"What should the agent do?"`.
“Run the research agent” is a valid prompt, so the tool **starts**. Task Manager
would have blocked on `topic` / `target` / `repo`+`pr_number`.

**B. JSON Schema `required` fights the prompt.** Prompt says: *If a required
argument is missing, call the tool anyway.* Native function-calling will not
omit required fields. The model invents a prompt or asks in prose. Prose does
**not** write `pending_action`.

**C. Slot key ≠ argument name.** `clarifyFromMatch` / `notFoundClarify` set
`Missing: []string{kind}` (`"agent"`, `"workflow"`, `"execution"`, …).
`mergePendingArgs` writes `args["agent"]`. `agent_execute` reads `strArg(input, "name")`.
Chip “Research Agent” does not fill `name`. `task_create` was patched
(`clar.Missing = []string{"project"}`); agents were not.
`TestAgent_ClarifyPending` only asserts pending is stored, not that the next
turn fills the real field.

**D. Specialists are catalog-gated.** `research_run`, `article_generate`,
`pentest_start` know `topic`/`target`. They are **not** in `AlwaysLoad`
(`platformtools/catalog.go`). The model’s default is `agent_execute`.
`review_pull_request` and `image_generate` **are** always-load.

**E. Execute is synchronous.** `ExecutionService.Execute` creates a row then
**blocks on `runner.Run`**. Chat wraps tools in **60s** (`agent.go` `executeTool`);
`research_run` / `article_generate` / `image_generate` get 3 minutes. Long
research/write/pentest via `agent_execute` times out even when slots are complete.

**F. Chat uses the org default model.** `chatagent.New(llmRouter.Default(), …)`.
`SupportsTools()` on Ollama probes **`DefaultModel`**, not `GenerateRequest.Model`.
`nativeCaller.next` does **not** set `GenerateRequest.Model`. Deployed default is
`llama3:latest`, which advertises **no `tools` capability** → ReAct JSON-in-prompt.

**G. `execution_get` ignores `last_execution`.** Description says use context when
id is omitted; implementation asks for an id and stores missing as `"execution"`
not `"execution_id"`. Empty pentest target is **refused** (`pentestTargetAllowed`
returns false for blank), not clarified.

**H. `/chat/route`** passes empty metadata and no history. Slot-fill cannot
survive a turn there. Do not break the session path to fix the stateless path.
Optional: document that `/chat/route` remains one-shot; do not invent session
state for it in this work unless a phase says so.

### Live Ollama inventory (workstation, 2026-08-27)

Host: Mac Studio, 128GB unified, Ollama 0.32.9, `https://ollama.workstation.co.uk`.

| Model | Tools | Use for chat? |
|---|---|---|
| `qwen3-coder:30b` (same digest as `:latest`) | yes | **Primary** |
| `llama3.1:8b` | yes | **Fallback** (already often loaded) |
| `llama3:latest` | **no** | **Forbidden for chat** (current Helm default) |
| `qwen3:30b-a3b` | yes + thinking | No (thinking tax; chat already sends `think: false`) |
| `muse-glimmer:latest` | yes + thinking + vision | No (wrong job; already occupies ~18GB) |
| `minicpm-v:latest` | vision only | No |
| `all-minilm:latest` | embedding | Recall only, not chat generate |
| mflux `:11435` | images | Not chat |

Review-bot Helm already uses `qwen3-coder:30b`. Do not change that unless it
breaks. Do **not** change `OLLAMA_DEFAULT_MODEL` for workers/python-sidecar as
the way to “fix chat” — chat must be a **separate** model pin.

---

## NON-NEGOTIABLES

1. A claimed action still requires a real tool run and a real ID (execution,
   article run, pentest run, review run). Fabricated tool XML / JSON in prose is
   still a failure (`ContainsToolScaffolding`).
2. No user-facing HTTP verbs, URL paths, curl, or raw UUIDs (existing sanitise).
3. Org scope from identity context, never from an LLM-supplied `org_id`.
4. Tool results stay wrapped `BEGIN_UNTRUSTED_TOOL_RESULT` … `END`.
5. Destructive tools still go through confirmation.
6. Do not pin workers to the chat model. Do not use `llama3:latest` as chat
   primary or fallback.
7. Keep `think: false` on Ollama chat calls.
8. Prefer extending existing packages (`chatagent`, `platformtools`, `llm`,
   `chatsvc`, `config`) over new frameworks.

---

## PHASE ORDER (do not skip)

| Phase | What | Why this order |
|---|---|---|
| **1** | Slot-key merge + tests | Chip answers work even before a better model |
| **2** | Partial JSON Schema (empty `required`, validate in `Run`) | Native tools will actually call with holes |
| **3** | Server-side agent input schemas + interview in `agent_execute` | The follow-up product bug |
| **4** | Specialist dispatch + async execute + `execution_get` last entity | Correct launch path; no 60s hang |
| **5** | Chat model routing (`CHAT_MODEL` / fallback) + Helm/env | Tool-capable default; workers unchanged |
| **6** | Persist tool transcript | Next turn sees what was already called |
| **7** | Harden pending / abandon / incomplete merge | Conversation edge cases |
| **8** | Chat `num_ctx` + window budget | Match coder without requesting 262k |

Phases 1–4 make execute **correct** on `llama3.1:8b`. Phase 5 makes the default
model one that can call tools. Do not ship phase 5 alone (“switch to coder”)
without 1–4 — the model will still invent prompts.

---

## PHASE 1 — Slot keys are real argument names

### Bug

`clarifyFromMatch(kind, …)` and `notFoundClarify(kind, …)` set
`Missing: []string{kind}`. `handlePending` / `mergePendingArgs` copy the user
reply onto that key. Tools read different keys.

### Mapping (fix every caller, not only agents)

| Clarify `kind` used today | Field the tool actually reads | Tools |
|---|---|---|
| `agent` | `name` (or `agent` only if that is the schema key — `schedule_create` uses `agent`) | `agent_execute`, `agent_get`, `execution_list`, `goal_get`, workflow agent pickers, insight/config tools that read `name` |
| `workflow` | `name` | `workflow_run`, `workflow_get` |
| `project` | `project` | `task_create` (already rewritten — keep it) |
| `task` | whatever the tool reads (`title` vs `name`) — **read the `Run` function** | `task_get`, etc. |
| `execution` | `execution_id` | `execution_get`, `execution_cancel` |
| `article_run` | `run_id` | `article_run_get` / publish / cancel |
| `pentest_run` | `run_id` | `pentest_findings` / cancel |
| `review_run` | `run_id` | `review_run_get` |
| `workflow_run` | `run_id` | `workflow_run_get` |
| `approval` | `approval_id` | `approval_decide` |

`schedule_create` and similar tools that **literally** take `"agent"` as the
JSON key must keep `Missing: ["agent"]`. The rule is: **`Missing` is the
`strArg` / schema property name**, not the English noun.

### Implementation

- Change `clarifyFromMatch` / `notFoundClarify` to take the **slot field name**
  (e.g. `clarifyFromMatch(kind, query, slot, candidates, nameOf)` or a small
  options struct). `kind` remains for the question text (“which agent”).
- `agent_execute` unnamed + many agents: `Missing: []string{"name"}`, question
  “Which agent should handle that?”, options = agent names. Same for “no
  agents yet” if you still use a slot — do not use `"agent"` unless the schema
  field is `agent`.
- `slotNames` in `agent.go` already uses `ClarifyRequest.Slot` (first missing).
  Ensure `executeTool` sets `Slot` from `res.Missing[0]` after the key fix so
  pending and SSE agree.

### Tests (required)

- **Full loop, not only pending stored:** scripted LLM calls `agent_execute`
  with `prompt` set and **no** `name` → clarify options include an agent name →
  second `Run` with that name as the user message → tool `Run` receives
  `name` equal to that string. Use a fake tool/registry like
  `TestAgent_ClarifyPending`.
- Same for `execution_get` missing id → user supplies UUID → `execution_id` set.
- Regression: `task_create` missing project still fills `project`.

Do not consider phase 1 done if only the helper comment changed.

---

## PHASE 2 — Partial tool calls are legal on the wire

### Bug

`tools.ObjectSchema(props, "prompt")` emits `"required": ["prompt"]`. Native
tool-calling (Ollama tools, OpenAI, Claude) will not call with an empty
required field. The system prompt’s “call anyway with what you have” is dead.

### Implementation

For **slot-fill tools** (anything that returns `Result{Missing, Question}`):

- Pass **no** required fields into `ObjectSchema` (empty `required` array). Keep
  properties + descriptions so the model knows the names.
- Inside `Run`, if a needed value is empty, return `Missing` + `Question` +
  `Options` when there is a closed set.

Minimum set:

- `agent_execute` — do not require `prompt` on the schema; if prompt empty
  **and** agent not yet resolved, still allow the call (phase 3 will interview).
- `research_run` — `topic` not schema-required; empty → `Missing: ["topic"]`,
  “What should I research?”
- `article_generate` — same for `topic`.
- `pentest_start` — `target` not schema-required; empty → clarify, **not**
  `pentestTargetAllowed` refuse. Refuse only when a **non-empty** target is
  out of scope.
- `review_pull_request` — `repo` / `pr_number` not schema-required; clarify
  each missing slot (existing pattern if any).
- `task_create` — keep runtime checks; `title` may stay required in schema **or**
  move to runtime; do not regress the project clarify path.

Do **not** strip `required` from tools that must not be called empty if they
have no clarify path (e.g. some deletes that already confirm). Prefer adding
clarify over leaving `required` for those.

Update tool descriptions: “Omit unknown fields; the tool will ask.”

### Tests

- Tool schema JSON for `agent_execute` has `"required": []` (or omitted empty).
- Direct `Run` with `{}` or `{name: "Research Agent"}` returns `Missing`
  containing the next slot (after phase 3: `topic` for researcher), not an
  execution.
- Native-schema unit test: `ObjectSchema` with zero required still emits a
  deterministic `required` array (existing `ObjectSchema` already does).

---

## PHASE 3 — One input contract: interview then launch

### Product rule

Chat and Task Manager ask for the **same required fields**. Port
`web/nextjs/lib/agents/input-schemas.ts` onto the server. Do not keep two
divergent lists.

### Server schema

Add a small Go module, e.g. `server/internal/agentschema` (name as you like),
used by platform tools only:

| Builtin (`metadata.builtin`) | Required slots (order) | Optional | Launch |
|---|---|---|---|
| `article_writer` | `topic` | `context`, `model` | `article_generate` / blog generate API |
| `researcher` | `topic` | `context` | `research_run` |
| `pentester` | `target` | `scan_mode` (default `quick`), `instruction`, `max_budget` | `pentest_start` |
| `pr_reviewer` | `repo`, `pr_number` | `dry_run` (default true in chat if that is already the tool default) | `review_pull_request` |
| `mail` | (sync/list drafts — do not dump into `agent_execute` prompt) | | `mail_sync` / `mail_list_drafts` |
| other / missing builtin | `prompt` (min length: treat a copy of “run the X agent” as empty) | | generic execute (async, phase 4) |

Field names must match Task Manager keys (`topic`, `target`, `scan_mode`,
`repo`, `pr_number`, …) so merge and chips stay consistent.

### `agent_execute` algorithm

1. Resolve agent:
   - `name` set → existing `resolveAgent` (clarify on mismatch; **slot `name`**).
   - `name` empty, 0 agents → clarify / create-first.
   - `name` empty, 1 agent → use it.
   - `name` empty, many → clarify `name` with options (do **not** auto-pick
     among five builtins).
2. Load schema from `agent.IsBuiltin(...)`.
3. Map incoming args: `prompt` may seed `topic` only if it is **not** a
   tautology of the execute request (e.g. reject prompts that equal / prefix
   “run the … agent” with no extra substance). Prefer dedicated fields.
4. For each required field still empty, return **one** `Missing` + question
   (sequential slot-fill). Free-text slots (`topic`, `target`) have **no**
   chips; closed sets (`scan_mode`, which agent) have `Options`.
5. When the schema is complete, **do not** dump a vague prompt into
   `Exec.Execute` for specialists. Call the same service the specialist tool
   uses (phase 4 may make `agent_execute` a dispatcher into those `Run`
   functions to avoid duplication).
6. Generic agents: require a real `prompt` (what to do). If missing, ask
   “What should {agent} do?”

### Pentest

Blank target → `Missing: ["target"]`, “What URL or path should I test?”
Non-empty + not allowlisted → existing refuse payload. Do not start a scan
without a filled target.

### Prompt

Keep “call the tool with what you have; do not guess.” After phase 2 this is
possible. Add: “Do not invent a topic, target, repo, or PR number.”

### Tests

- Builtin researcher + `agent_execute{name: "Research Agent"}` → question about
  topic, `pending` missing `topic`, **no** `Exec.Execute`.
- Next user message “Kubernetes cost optimisation” → research path invoked with
  that topic (mock research service).
- Pentester + no target → clarify target, not `refused`.
- PR reviewer + repo only → clarify `pr_number`.
- Generic custom agent + name + no prompt → clarify prompt.
- Frontend `input-schemas.ts` stays the UI source; if you must duplicate,
  add a comment on both files: “keep in sync with agentschema”. Prefer one
  shared list of keys even if types stay in Go vs TS.

---

## PHASE 4 — Specialist tools + async execute

### 4a. Routing

Either:

- Add `research_run`, `article_generate`, `pentest_start` to `AlwaysLoad`, **or**
- After interview, `agent_execute` internally calls those tools’ `Run` functions.

Prefer **dispatch from `agent_execute`** plus **always-load the three
specialists** so the model can also call them directly. Update `HumanLabel`.
`mail_sync` / `mail_list_drafts`: always-load or dispatch from mail builtin;
prompt already names them.

`review_pull_request` is already always-load. “Run the PR reviewer” should
interview repo/PR then call that tool, not generic execute.

### 4b. Async generic execute

`ExecutionService.Execute` **blocks until `runner.Run` finishes**. Chat must
not wait for a full worker.

Add something like `Start(ctx, orgID, agentID, req) (*model.AgentExecution, error)`
that:

- Creates the execution row, marks started, kicks `runner.Run` on a
  **background context** (not the 60s tool context), returns immediately with
  status running/pending.
- Reuse persist/usage logic from `Execute` in the goroutine; log failures;
  restore agent status as today.

`agent_execute` (generic path) calls `Start`, returns
`{agent, status, execution implied via Entity}`. Message: work has started,
not “here is the full answer.”

Chat `executeTool` timeout for `agent_execute` can stay ~60s for **start**,
which should be milliseconds. Do not keep blocking `Execute` on this path.

Specialist tools that already return a run id (`article_generate`,
`pentest_start`, `review_pull_request`) already start work; leave them, but
do not also call blocking `Execute`.

`research_run` today may still be sync inside the 3m timeout. If it cannot
return quickly, start-and-poll is in scope; if it already returns a brief,
keep it but do not route researcher through blocking `agent_execute`.

### 4c. `execution_get`

When `execution_id` omitted, use session `last_execution` from
`readEntities` / identity — **the tool must receive entities**. If the loop
does not pass session entities into tools today, pass them via context
(same pattern as `WithIdentity`) or have `agent_execute` always return an
entity and teach the prompt to call `execution_get` with that id from the
tool result in the **same turn** if status is not terminal.

Minimum: omitted id + `last_execution` in session metadata → look up that id.
If neither → `Missing: ["execution_id"]`.

### Tests

- Generic execute returns while a fake runner is still sleeping > 2s (status
  running, entity id set).
- `execution_get` with empty id uses last execution in a chat turn after start.
- Specialist researcher does not call blocking `Execute`.

---

## PHASE 5 — Chat model routing

### Config (new; do not overload worker default)

| Env | Default | Meaning |
|---|---|---|
| `CHAT_MODEL` | `qwen3-coder:30b` | Primary chat generate model |
| `CHAT_MODEL_FALLBACK` | `llama3.1:8b` | If primary generate fails (error, timeout, empty) **one** retry |
| `CHAT_NUM_CTX` | `16384` | Ollama `num_ctx` for **chat client only** (phase 8 may tune) |

`OLLAMA_DEFAULT_MODEL` / Helm `ai.ollamaModel` stay as they are for workers
until operators choose otherwise. **Do not** set them to `qwen3-coder:30b`
as the only change.

`LLM_PROVIDER` still selects the provider client family (ollama / openai /
claude). Chat model names apply when the default provider is Ollama.
If `LLM_PROVIDER=openai`, chat uses `OPENAI_DEFAULT_MODEL` unless you add
`CHAT_MODEL` as a generic override — implement: when `CHAT_MODEL` is set,
`GenerateRequest.Model = CHAT_MODEL` on the chat client regardless of
provider, with fallback only for Ollama (cloud fallback is out of scope
unless trivial).

### Why a dedicated client

`OllamaClient.SupportsTools()` uses `c.DefaultModel`.
`turnCallerFor` chooses native vs ReAct from that.
`nativeCaller` does not set `GenerateRequest.Model`.

Therefore:

1. Build a **chat client** whose `DefaultModel` is `CHAT_MODEL` (clone/wrap
   the Ollama client from the router; same base URL, JWT, timeout).
2. `SupportsTools()` must reflect **the chat model**, not `llama3:latest`.
3. Set `GenerateRequest.Model` on every chat `Generate` (native and ReAct)
   to the active model (primary, or fallback after failure).
4. Fallback: on primary `Generate` error, retry once with
   `CHAT_MODEL_FALLBACK`. Log `chatagent: falling back to llama3.1:8b`.
   If fallback also lacks tools, ReAct is allowed; if fallback **has** tools,
   use native. **Never** fall back to `llama3:latest` or empty model.
5. Refuse to start chat on a forbidden model: if `CHAT_MODEL` is
   `llama3`, `llama3:latest`, `minicpm-v`, or `muse-glimmer`, log error and
   substitute primary default `qwen3-coder:30b` (or fallback if coder unset).

Wire: `chatagent.New(chatClient, …)` not `llmRouter.Default()`.
Langfuse wrap the chat client the same way as other clients if wrapping is
global — chat must still report `SupportsTools` for coder / llama3.1.

### Helm / compose / example env

- `deploy/helm/jobshout/values.yaml`: `ai.chatModel`, `ai.chatModelFallback`,
  `ai.chatNumCtx`.
- Ring overlays (`values-int.yaml`, `values-test.yaml`, `values-acc.yaml`,
  `values-prod.yaml`): set `chatModel: qwen3-coder:30b`,
  `chatModelFallback: llama3.1:8b`. Leave `ollamaModel: llama3:latest` unless
  you have an explicit operator request to change workers.
- `templates/configmap.yaml`: `CHAT_MODEL`, `CHAT_MODEL_FALLBACK`, `CHAT_NUM_CTX`.
- `docker-compose.yml` jobshout-server env, `.env.example`,
  `server/internal/config/config.go` viper defaults.
- Do not change python-sidecar `OLLAMA_DEFAULT_MODEL` in this phase.

### Tests

- With a fake router, chat agent’s client default model is `CHAT_MODEL`.
- `SupportsTools` true when chat model is a tools model even if org default
  is llama3.
- Generate failure on primary → second Generate uses fallback model name.
- Forbidden model names are not sent.

### modelselect catalog (optional but useful)

`server/internal/modelselect/catalog.go` still lists `llama3:latest` with
`SupportsTools: false` only. Add `qwen3-coder:30b` and `llama3.1:8b` with
`SupportsTools: true` so auto-select for **workers** can use them. Do not
make chat go through `modelselect` unless it is a thin helper; chat routing
is explicit env, not task-kind scoring.

---

## PHASE 6 — Persist the tool transcript

### Bug

`chatsvc` appends user + assistant **prose**. Next `Run` history cannot see
prior tool calls. Pending/entities/summary are the only bridge. Prose
clarify has no pending row.

### Implementation

Within a turn, after the loop (or incrementally): persist:

- Assistant messages that contain `ToolCalls` (store args in
  `ChatMessage.Metadata`, role `agent` or a dedicated representation
  `toLLMHistory` already maps `ChatRoleTool` → `llm.RoleTool`).
- Tool result messages: `Role: ChatRoleTool`, content = wrapped payload
  **truncated** (e.g. 4k chars) so the window cannot explode.

`toLLMHistory` must restore `ToolCalls` / `ToolCallID` from metadata so the
next native turn is valid OpenAI/Ollama/Claude history.

Windowing: include these rows in the token budget. Evict oldest first
(already newest-from-the-end). Truncate huge tool blobs in `Window` or at
write time.

Do not persist secrets (existing `stripSecretArgs`).

UI: do not dump raw tool JSON in bubbles unless already shown via
`ActionRecord` / chips. History API may return extra rows; MessageList
should ignore `role=tool` or render compactly. **Do not break** the current
user/agent bubble layout.

### Tests

- After a turn that called `task_create` then finished, `ListMessages`
  includes a tool-result row (or metadata on agent message that
  `toLLMHistory` expands). Second `Run` with that history sends a tool-role
  message to the fake LLM.
- Truncation: a 50k tool payload is not stored in full.

---

## PHASE 7 — Harden the pending loop

### Bugs

- `isAbandon`: `help`, prefix `show me`, prefix `list ` cancel pending.
  Easy to drop a slot by accident.
- `handlePending` always merges and re-runs the tool. A new unrelated
  request still fills the old slot.
- After merge, if required slots are still empty, some tools execute
  anyway (generic prompt). Must **re-clarify**, not launch.
- Confirmation is checked **before** pending slot-fill. Document and add a
  test so a confirm wait is not overwritten by a slot merge; if both exist,
  confirm wins (current order) — just don’t lose pending if the user says
  “yes” to confirm. Do not invent a new race unless tests show one.

### Implementation

- Abandon only: explicit negatives (`isNegative`), “never mind” /
  “forget it” / “forget that”, and `actually ` **only if** you keep it
  (prefer requiring “never mind” for abandon). Remove `help`, `show me`,
  `list ` from `isAbandon`.
- If pending exists and the message is **not** abandon: if it looks like a
  new imperative (optional heuristic: starts with list/create/run and
  pending is for a different tool) → clear pending and `loop`. Keep this
  conservative; tests for “Which project?” + “Website” still merge.
- After merge, if the tool returns `Missing` again → stay in clarify
  (already mostly true). If the tool **succeeds** with a tautological
  prompt, phase 3 should have blocked; add a guard on generic execute:
  prompt must fail a “too thin” check (length + not equal to the original
  execute utterance stored in pending args).
- If merge leaves the slot empty (whitespace) → ask again, do not execute.

### Tests

- Pending project + user “list my agents” → **does not** set project to
  that sentence; either abandon-removed or LLM path. After abandon change,
  this should go to `loop` (list agents), not `task_create`.
- Pending + “Website” still completes `task_create`.
- Pending topic + `"   "` → clarify again.

---

## PHASE 8 — Context window for chat

### Facts

Coder’s architecture is 262k; workstation `ollama ps` often loads
**8192**. Global `OLLAMA_NUM_CTX` is 8192. Chat system prompt + always-load
schemas + 4k history need more than 8k once transcripts exist, but
requesting 262k is hostile on a shared GPU.

### Implementation

- Chat client: `WithNumCtx(cfg.ChatNumCtx)` default **16384**. Cap to the
  model’s reported context via existing `effectiveNumCtx` (do not exceed
  model limit).
- Optionally raise `windowTokenBudget` from 4000 to **6000** after
  transcripts exist; keep `minTurns = 4`.
- Do not change worker `OLLAMA_NUM_CTX` in this phase unless chat and
  workers share one client — they must not.

### Tests

- Chat Ollama requests include `num_ctx` 16384 (or configured) for the
  chat model.
- `effectiveNumCtx` still clamps to model max.

---

## ACCEPTANCE (manual + automated)

Automated: all new tests above; `go test ./internal/chatagent/... ./internal/platformtools/... ./internal/llm/... ./internal/chatsvc/... ./internal/config/...` (and agentschema / execution as touched).

Manual (session chat, not `/chat/route`):

1. “Run the research agent” → **asks for a topic**, does not start a run.
   Reply with a topic → research starts (or async status), not a 60s timeout
   with an empty brief.
2. Chip “which agent?” then pick Research Agent → `name` is set; then topic
   interview (not a second which-agent loop).
3. “Run the pentester” → asks for target; blank never scans.
4. “Create a task to fix login” → still asks project if multiple.
5. Logs / usage: chat `model` is `qwen3-coder:30b` (or fallback after a
   forced primary error in a test).
6. Reply that claims execute includes an `actions[]` entry and a real
   execution/run id in entities.

---

## OUT OF SCOPE

- Rewriting the web chat UI (except ignore `role=tool` in the list if needed).
- Stateless `/chat/route` session memory.
- Replacing ReAct for models that truly have no tools (fallback `llama3.1:8b`
  has tools).
- Changing mflux / image models / embeddings.
- Forcing `OLLAMA_DEFAULT_MODEL=qwen3-coder:30b` for the whole platform.
- Multi-agent planner inside chat.

---

## IMPLEMENTATION NOTES

- Match existing Go style: small files, table tests, no panic in tools.
- `platformtools.Result.Missing` is the contract pending merge uses — keep it
  aligned with schema property names after phase 1.
- `main.go` chat construction today:

  `chatAgent := chatagent.New(llmRouter.Default(), platformReg, chatGuard, memorySvc, logger)`

  Replace the first argument with the chat client from phase 5.
- If `Exec.Start` needs a new interface method, add it to
  `ExecutionService` and the fake in tests; do not call HTTP from tools.
- Commit in phase-sized PRs if the team splits work; if one branch, keep
  commits grouped by phase.

When finished, the control desk still tool-calls; it **interviews**, **does not
guess specialist fields**, **starts work asynchronously**, and **talks to
`qwen3-coder:30b`** with **`llama3.1:8b` as the only fallback**.
