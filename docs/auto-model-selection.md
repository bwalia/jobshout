# Cursor-style "Auto" model selection for JobShout agent tasks

## Status

**Landed:** the `internal/modelselect` package (candidate catalog, scoring, filtering,
fallback ordering), the `"auto"` sentinel, wiring into both the ReAct executor and the
autonomous planner/reflector, the `AUTO_MODEL_SELECTION` config flag, and chart plumbing.

**Not yet built** — deliberately deferred, each worth its own change:

- Persisting the decision (provider/model/reason) on the execution record. Needs
  migration `000019`, a model field, and repository plumbing. Today the reason is
  logged, not stored, so the API cannot yet show a user *why* Auto chose what it did.
- Feeding governance policy into `Constraints`. The selector honours allowlists and a
  budget cap, but the executor currently passes empty `Constraints` — `EnforcePolicy`
  still runs separately in `execution_service.go` against the agent's declared model.
- Acting on `Fallbacks` at the call site. The chain is computed and returned, but no
  caller retries down it yet.

The original implementation prompt follows.

## Goal

Today every agent execution pins a provider and model up front. Add an **Auto mode**
that picks the best provider+model *per task* at execution time — cheap/fast models for
simple work, stronger models for hard reasoning, tool-capable models when the task needs
tools — while staying inside governance policy and cost budget.

Auto must be the comfortable default for most tasks, and must never be worse than the
current behaviour: an agent with an explicit provider/model keeps getting exactly that.

## Where this lives (read these first)

- `server/internal/llm/router.go` — `Router.For(providerName) (Client, error)`; registers
  `ollama` always, `openai`/`claude` only when an API key is set. `RegisteredProviders()`
  returns what is actually available. **Auto may only choose among registered providers.**
- `server/internal/llm/client.go` — the `Client` interface (`Generate`, `ProviderName`),
  `GenerateRequest{Messages, Model, MaxTokens, Temperature, ToolDefs}`,
  `GenerateResponse{Content, FinishReason, InputTokens, OutputTokens, ToolCalls}`, and the
  optional `ToolCapableClient{SupportsTools() bool}` capability interface.
- `server/internal/model/agent.go:23-24` — `ModelProvider *string`, `ModelName *string`.
  `nil` currently means "fall back to the configured default".
- `server/internal/executor/autonomous.go:300` — `resolveClient(agent)` calls
  `a.llm.For(providerName)`. This is the main hook point.
- `server/internal/service/execution_service.go:55-95` — resolves provider/model, calls
  `govSvc.EnforcePolicy(ctx, orgID, agentID, provider, modelName)`, then
  `engine.ResolveEngine(agent, req.EngineOverride, "")`.
- `server/internal/costengine/costengine.go` — `Calculate(provider, model, inputTokens,
  outputTokens, latencyMs) float64`, catalog keyed `"provider:model"` with a `"provider:*"`
  fallback. Use this for cost-aware scoring; do not invent a second pricing table.
- `server/internal/engine/router.go:61` — `ResolveEngine`, the existing precedence pattern
  (agent → request override → step override). Mirror this style.

## What to build

### 1. A selector package

Create `server/internal/llm/autoselect` (or `server/internal/modelselect` if that reads
better) exposing something like:

```go
type TaskSignals struct {
    Kind          string   // "plan" | "step" | "chat" | "summarize" | "classify" | "code"
    PromptTokens  int      // estimated
    NeedsTools    bool     // ToolDefs will be sent
    NeedsJSON     bool     // structured output expected
    StepIndex     int      // position in a multi-step plan
    PriorFailures int      // retries so far, for escalation
}

type Decision struct {
    Provider string
    Model    string
    Reason   string   // human-readable, persisted for observability
    Fallbacks []Candidate
}

type Selector interface {
    Select(ctx context.Context, sig TaskSignals, allowed Constraints) (Decision, error)
}
```

`Constraints` carries what governance and configuration permit: allowed providers, allowed
models, max USD per call, and whether tool-calling is required.

### 2. A capability catalog

A declarative table of candidate models with: provider, model id, context window, whether
it supports native tools, a quality tier, and a speed tier. Pricing must come from
`costengine`, not be duplicated here. Keep the catalog in one file, easy to edit, and
**data-driven — no `if provider == "openai"` chains in the selection logic.**

Skip any candidate whose provider is not registered in the `Router`, and any candidate
lacking tool support when `NeedsTools` is true (check via the `ToolCapableClient`
type-assertion, matching how the codebase already probes that capability).

### 3. Wire "auto" in as a sentinel

Treat `ModelProvider == "auto"` (and/or a new `Auto bool`) as the trigger. Preserve
today's semantics exactly:

- explicit provider + model → unchanged, selector not consulted
- explicit provider, no model → unchanged
- `"auto"` → selector runs
- `nil` → **unchanged for now** (configured default). Do not silently flip existing agents
  into Auto; make that an explicit opt-in via config or a follow-up migration.

### 4. Governance and budget are hard gates, not hints

Run `govSvc.EnforcePolicy` against the *selected* provider/model, not just the agent's
declared one. If the selection violates policy, fall back down the candidate list rather
than failing the execution — and if nothing passes, return a clear error naming the policy
that blocked it.

### 5. Fallback chain

On provider error (rate limit, 5xx, unregistered, context-length exceeded), retry with the
next candidate. Bound the attempts. Log each hop. A context-length failure should escalate
to a larger-context candidate specifically, not just the next one in line.

### 6. Persist the decision

Record the chosen provider, model, and `Reason` on the execution record so a user can see
*why* Auto picked what it did. Add a migration — the next number is **`000019`** (latest is
`000018_approvals`), with matching `.up.sql` and `.down.sql`.

### 7. Config

Add viper settings in `server/internal/config/config.go` following the existing
`viper.SetDefault` pattern in `Load()`, e.g. `AUTO_MODEL_SELECTION_ENABLED`,
`AUTO_MODEL_MAX_USD_PER_CALL`, `AUTO_MODEL_DEFAULT_TIER`. Surface anything the deployment
needs through `deploy/helm/jobshout/templates/configmap.yaml` and `values.yaml`.

## Non-goals

- No new HTTP provider integrations — work with `ollama`, `openai`, `claude` as registered.
- No changes to the embedder path (`EmbedderFor`); embeddings stay separately configured.
- No automatic model switching *mid-generation*; selection happens once per call, plus the
  fallback chain on error.
- Do not touch the approvals flow's `ModelName` plumbing beyond recording the chosen model.

## Repo conventions to follow

- Go, standard-library table tests. **The repo does not use testify** — match the style in
  `server/internal/costengine/costengine_test.go`.
- Tests must be deterministic and must not hit the network. There is an existing test stub
  pattern in `server/internal/llm/testing.go` — use or extend it.
- `gofmt` your new files. Several existing files have pre-existing formatting drift
  (`cmd/server/main.go`, `internal/config/config.go`) — **do not reformat unrelated code**;
  keep the diff to this feature.
- Run `go build ./...`, `go vet ./...`, and `go test ./...` from `server/`.

## Acceptance criteria

1. An agent with `ModelProvider="auto"` executes end-to-end and picks a sensible model.
2. An agent with an explicit provider/model behaves **byte-identically to today**.
3. A task requiring tools never selects a model that lacks tool support.
4. A provider that is not registered (no API key) is never selected.
5. Governance policy is enforced against the selected model; a blocked selection falls
   through to the next candidate rather than erroring out.
6. Provider failure falls back down the chain; context-length failure escalates to a
   larger-context candidate.
7. The chosen provider, model, and reason are persisted and visible on the execution.
8. Tests cover: signal→decision mapping, tool-capability filtering, unregistered-provider
   filtering, governance rejection, budget rejection, and the fallback chain.
9. `go build`, `go vet`, and the full `go test ./...` all pass.

## Deliverable

Start by proposing the candidate catalog schema and the scoring rule, and confirm the
sentinel approach for "auto" before writing the wiring — those two decisions shape
everything else.
