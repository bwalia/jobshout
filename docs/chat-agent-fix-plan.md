# Chat Agent Fix Plan

> Execution plan for fixing the defects found in `docs/chat-agent-eval-report.html`
> (published audit, 2026-08-24). Self-contained: a fresh session can execute this
> without any other context. Work on branch `feat/chat-agent-upgrade`.
>
> Environment: API runs via `./start-local.sh` on **:8181** (NOT 8080 — that port is
> taken by Ring Promoter), UI on :3001, native Postgres on :5432. LLM is
> `LLM_PROVIDER=ollama` with `qwen3-coder:30b` behind a JWT gateway (see `.env`).
> Logs in `.dev-logs/server.log`.

## The diagnosis (verified, do not re-derive)

The rebuilt chat agent (`server/internal/chatagent/`, `server/internal/platformtools/`,
74 tools) never executes a single tool on this deployment. 15/15 live probes ran zero
tools; 13 replies contained a **fabricated** tool result with invented data, including
`"status": "success"` for a task-create that never happened. Three facts combine:

1. `OllamaClient.SupportsTools()` hard-returns `false` (`server/internal/llm/ollama.go:112-114`)
   and the client has **no tools support at all** — no `tools` field in
   `ollamaChatRequest` (`ollama.go:117`), no `tool_calls` in `ollamaMessage` (`ollama.go:139`),
   and `Generate` drops `req.ToolDefs`, `Message.ToolCalls`, and `Message.ToolCallID`
   silently (`ollama.go:165-172`).
2. `chatagent.Agent.Run` sends `ToolDefs` unconditionally and treats
   `len(llmResp.ToolCalls) == 0` as "final answer" (`server/internal/chatagent/agent.go:262-283`).
   There is **no capability check and no ReAct fallback** — unlike the executor, which
   guards with `clientSupportsTools()` at `server/internal/executor/executor.go:235` and
   falls back to `reactLoop`.
3. The system prompt names the `BEGIN_UNTRUSTED_TOOL_RESULT` / `END_UNTRUSTED_TOOL_RESULT`
   delimiters (`server/internal/chatagent/prompt.go:20`), so the model — receiving tool
   *instructions* but no tool *mechanism* — autocompletes fake tool-result blocks in that
   exact shape. `SanitiseMessage` (`chatagent/sanitise.go:16`) has no rule for them, so the
   scaffolding renders verbatim in the UI.

Useful existing infrastructure (verified present):
- `llm.ToolCapableClient` interface (`llm/client.go:95-102`); Claude and OpenAI clients return true.
- Per-model capability cache on OllamaClient: `rememberModels`/`lookupModel`
  (`llm/ollama_models.go:111-146`), populated by `ListModels` from Ollama's `/api/tags`,
  which on Ollama ≥ 0.6 reports a `capabilities` list including `"tools"`.
  `ModelInfo.SupportsTools()` exists (`llm/models.go:66`). The comment block at
  `ollama_models.go:110` explicitly says tool support is per-MODEL and the provider-wide
  answer can't express it — this plan completes that thought.
- Executor's working reference implementations: `buildToolDefs`, `clientSupportsTools`,
  `runNative`, `reactLoop`, `parseReActResponse` (`executor/executor.go`).
- Chat loop internals that the fix must preserve: `executeTool` (guard chain),
  `holdConfirmation`, clarify/pending-action handling, `wrapToolResult` (agent.go:507),
  `disclosedFromResult` progressive disclosure, `writeEntities` session entity state.
- Wiring: `main.go:584-587` — `chatagent.New(llmRouter.Default(), platformReg, chatGuard, memorySvc, logger)`.

---

## Phase 1 — Native tool-calling in the Ollama client  (the real fix)

**File: `server/internal/llm/ollama.go`** (+ a new `ollama_tools_test.go`)

1. Extend the wire structs:
   - `ollamaChatRequest`: add `Tools []ollamaTool `json:"tools,omitempty"``.
   - New types mirroring Ollama's API:
     ```go
     type ollamaTool struct {
         Type     string             `json:"type"`               // always "function"
         Function ollamaToolFunction `json:"function"`
     }
     type ollamaToolFunction struct {
         Name        string         `json:"name"`
         Description string         `json:"description"`
         Parameters  map[string]any `json:"parameters"`
     }
     type ollamaToolCall struct {
         Function struct {
             Name      string         `json:"name"`
             Arguments map[string]any `json:"arguments"`  // Ollama returns a JSON object, not a string
         } `json:"function"`
     }
     ```
   - `ollamaMessage`: add `ToolCalls []ollamaToolCall `json:"tool_calls,omitempty"`` and
     `ToolName string `json:"tool_name,omitempty"``.

2. In `Generate`, translate the full message history (currently `ollama.go:171-174` copies
   only Role+Content):
   - Assistant messages with `m.ToolCalls`: echo them as `tool_calls` on the wire message.
   - `RoleTool` messages: send `role:"tool"`, `content`, and set `tool_name` from the
     original call (thread it — see step 4 note on IDs).
   - When `len(req.ToolDefs) > 0` **and** the resolved model supports tools
     (`c.lookupModel(model)` → `info.SupportsTools()`), attach `Tools` built from
     `req.ToolDefs`. When the model does not support tools, leave `Tools` empty — callers
     that guard properly (Phase 2) will not send ToolDefs to such a model anyway, but this
     keeps `Generate` safe for any caller.

3. In `readStream`, accumulate `chunk.Message.ToolCalls` across chunks (Ollama streams them
   in the NDJSON chunks; usually one chunk carries them all — append, don't overwrite).
   Map to `[]llm.ToolCall` on the returned `GenerateResponse`. Ollama does not supply call
   IDs, so synthesize stable ones: `fmt.Sprintf("call_%d", i)`. The chat loop echoes
   `ToolCallID` back on RoleTool messages purely for provider bookkeeping; for Ollama the
   `tool_name` field is what matters, so map ToolCallID → tool_name via the preceding
   assistant message's calls (order-preserving; keep it simple — walk the history when
   translating).

4. Replace the hard-coded capability answer:
   ```go
   func (c *OllamaClient) SupportsTools() bool {
       info, ok := c.lookupModel(c.DefaultModel)
       return ok && info.SupportsTools()
   }
   ```
   `lookupModel` only knows models after `ListModels` ran. Make it self-priming: on cache
   miss, call `c.ListModels(ctx)` once with a short timeout (e.g. 5s, background context)
   and re-check; on failure return false. **False stays the safe answer** — after Phase 2 a
   false negative costs only the ReAct fallback, exactly as the comment at
   `ollama_models.go:81-84` reasons.

5. Tests (`ollama_tools_test.go`, use `httptest` like the existing `ollama_stream_test.go`):
   - Request marshalling: ToolDefs → `tools` array present; assistant tool_calls and
     role:"tool"/tool_name round-trip.
   - Response parsing: NDJSON stream carrying `message.tool_calls` → `GenerateResponse.ToolCalls`
     with synthesized IDs and decoded argument maps.
   - `SupportsTools()`: true when /api/tags reports `capabilities:["completion","tools"]`
     for the default model; false when capabilities absent (pre-0.6 server).

## Phase 2 — Capability guard + ReAct fallback in the chat loop  (never fabricate by construction)

**Files: `server/internal/chatagent/agent.go`, new `server/internal/chatagent/react.go` (+ tests)**

1. Introduce a tiny seam so the rest of the loop (guard chain, confirmation, clarify,
   entities, disclosure) is shared between both modes:
   ```go
   // turnCaller produces the next model step: either tool calls or a final answer.
   type turnCaller interface {
       next(ctx context.Context, messages []llm.Message, defs []llm.ToolDef) (*llm.GenerateResponse, error)
   }
   ```
   - `nativeCaller` — exactly today's `a.client.Generate(... ToolDefs: defs ...)` call.
   - `reactCaller` — sends **no** ToolDefs. Instead it appends a protocol instruction to the
     system prompt (build once): the model must reply with ONLY one JSON object, either
     `{"tool":"<name>","args":{...}}` or `{"final":"<answer>"}`; render the available tools
     into the prompt from `defs` (name, description, compact schema). Parse with the
     tolerant JSON helpers in `llm/json.go` (same ones `parseReActResponse` relies on).
     A `tool` reply becomes a one-element `ToolCalls` slice (synthesize ID); a `final`
     reply becomes plain Content; an unparseable reply → one retry with a corrective
     system nudge, then treat as final text.
2. Select the caller once per `Run`, mirroring `executor.go:235`:
   ```go
   tc, ok := a.client.(llm.ToolCapableClient)
   native := ok && tc.SupportsTools()
   ```
   Log at Info on every session's first turn which mode is active; at startup
   (`main.go`, next to line 585) log a **warning** when the chat model lacks native tools:
   `"chat agent: model X has no native tool-calling — using ReAct fallback"`.
   Never silently proceed without either mode.
3. Do not change: `executeTool`, `holdConfirmation`, clarify/pending handling,
   `wrapToolResult`, disclosure, entity writing, usage accounting. They all key off
   `llm.ToolCall` values, which both callers now produce.
4. Tests (extend `agent_test.go`, which already uses a scripted fake client):
   - Fake client with `SupportsTools()==false` + scripted ReAct JSON → tool executes, action
     recorded, entities present.
   - Malformed ReAct JSON → retry then honest final answer, zero fabricated actions.
   - Native path unchanged (existing tests keep passing).

## Phase 3 — Honesty guards + sanitiser  (belt and braces)

**Files: `server/internal/chatagent/sanitise.go`, `agent.go` (+ tests)**

1. `SanitiseMessage`: strip fabricated/leaked tool scaffolding —
   - the whole block from `BEGIN_UNTRUSTED_TOOL_RESULT` to `END_UNTRUSTED_TOOL_RESULT`
     (and either marker appearing alone), regex with `(?s)`;
   - bare tool-call JSON shapes the model may emit as text: a line-anchored
     `^\s*\{\s*"(name|tool)"\s*:` … balanced-ish block — keep the regex conservative
     (delimiters + `"result"\s*:\s*\{` patterns) rather than trying to parse JSON.
2. Fabrication guard on the final-answer exit path (`agent.go:283-310`): after sanitising,
   if the turn executed **zero** tools (`len(actions)==0`) and the pre-sanitised text
   matched any scaffolding pattern above, replace the message with an honest failure
   (`"I couldn't complete that action — the tool step didn't run. Please try again."`),
   log at Error with the model name, and emit it as the response. With Phases 1–2 in place
   this should never fire; it exists so a regression is loud and harmless instead of
   silent and convincing.
3. Tests: table-driven sanitiser cases (delimiter block mid-message, marker alone,
   fabricated `"status":"success"` JSON); loop test asserting the guard fires and the
   fabricated text never reaches the response.

## Phase 4 — UI fixes

1. **Dock overlays the full chat page.** `web/nextjs/components/chat/ChatDock.tsx` has no
   route awareness and `app/(app)/layout.tsx:72` mounts it globally. In `ChatDock`, add
   `const pathname = usePathname()` and `if (pathname?.startsWith("/chat")) return null;`
   (also skip rendering the floating launcher button there).
2. **Real token streaming.** Today the loop emits ONE `EventToken` carrying the whole
   message (`agent.go:304`), so the UI cannot render progressive text.
   - Add `OnToken func(string)` to `llm.GenerateRequest` (tag as ignored for JSON; it's a
     process-local callback). `OllamaClient.readStream` invokes it per content chunk when
     set. Other clients ignore it — no behaviour change.
   - In `chatagent`, pass an `OnToken` that forwards chunks as `EventToken` stream events.
     Content that precedes tool calls ("Let me check…") streams too — that is normal chat
     UX. The `done` event still carries the authoritative sanitised envelope; the web
     client already replaces streamed text with `response.message` on `done` — verify in
     `web/nextjs/lib/api/chat.ts` / `useChat.ts` and keep that contract.
3. Frontend check: `cd web/nextjs && npx tsc --noEmit` and the chat Playwright spec
   (`e2e/chat.spec.ts`) if the suite runs locally.

## Phase 5 — Small cleanups

1. **`POST /chat/route` vestigial fields** (`server/internal/chatsvc/router.go`): it
   reported `intent:"help", confidence:1` on an agent-list reply. Derive `Intent` honestly
   from the envelope: `"confirm"` when Confirmation set, `"clarify"` when Clarify set,
   else first action's tool name, else `"chat"`; drop the fake confidence (omit or 0).
   Check the adapter's Route implementation and fix where it fabricates these.
2. Run the full server suite: `cd server && go test ./...`.

## Phase 6 — Re-run the evaluation

The audit measured a blocked system; everything downstream is unmeasured. After Phases 1–5:

1. Restart via `./start-local.sh` (or rebuild: `cd server && go build -o bin/jobshout-server ./cmd/server`, then restart the running process; check `.dev-logs/server.log` for the tool-mode startup line).
2. Register a fresh org (`POST /api/v1/auth/register`, field is `org_name`), seed the
   fixture (3 custom agents incl. the "Data"/"Database" trap pair, 1 project, 5 tasks,
   2 workflows — see the reproduction appendix in `docs/chat-agent-eval-report.html`).
3. Re-run at minimum these probes through `POST /api/v1/chat/sessions/{id}/messages`,
   verifying every claim against the REST API afterwards:
   - A1 "what agents do I have?" → real agent names, `actions[0].tool == "agent_list"`, entities non-null.
   - T2 "create a task to fix the login timeout, high priority, in the Platform project"
     → task exists in `GET /api/v1/tasks`; envelope has a task entity with real ID.
   - A7 "ask the DevOps agent to check the staging health endpoint" → an execution row appears.
   - W1 "what workflows exist?" → the two real workflows, no inventions.
   - X4 "run Data" (with both "Data Agent" and "Database" present) → a clarify/disambiguation, not a silent pick.
   - X5 "delete the DevOps agent" → `pending_confirmation` + Confirmation envelope, agent NOT deleted until approved.
   - M1 "run a health check with the DevOps agent" → "now do the same for staging" → second turn reuses the agent (check `session.metadata.entities`).
4. UI smoke in Chrome: dock absent on `/chat`, tool chips + entity cards render from
   `actions[]`/`entities[]`, streamed text arrives progressively, no
   `BEGIN_UNTRUSTED_TOOL_RESULT` anywhere.
5. Update `docs/chat-agent-eval-report.html` — republish to the SAME artifact URL
   (https://claude.ai/code/artifact/aea07a93-671a-4e4b-8351-0f0d37768d05) with the re-run results.

## Acceptance criteria

- With `LLM_PROVIDER=ollama` + `qwen3-coder:30b`: A1/T2/A7/W1 execute real tools, verified out-of-band.
- With a model lacking native tools (or capability lookup failing): ReAct fallback engages,
  tools still execute; a startup warning names the mode. In no configuration does the agent
  answer action-shaped requests without a tool mechanism.
- No response on any surface contains the untrusted-result delimiters or fabricated tool JSON.
- Zero-action turns never claim completed work (guard test proves it).
- `/chat` page shows exactly one chat UI. Streaming shows progressive tokens.
- `go test ./...` green; `npx tsc --noEmit` green; existing executor/ReAct behaviour untouched.

## Order & discipline

Work the phases in order — 1 and 2 are independent enough to build together but test 1
first (it's the deployment's actual fix; 2 is the structural guarantee). Commit per phase
with focused messages. Don't refactor beyond the seam described in Phase 2; the guard
chain, memory, and envelope code is untested-in-production but unit-tested — leave its
shape alone until the re-run (Phase 6) says otherwise.
