# Chatbot — Plan

## Purpose

Let the user access and execute all agents through chat. Every agent execution must go through Task Manager so it appears on the board.

## Current behaviour

`chatagent.Agent.Run` + platform tools. Default model `qwen3-coder:30b`. Native tools + ReAct fallback exist. Memory: 6k-token window, rolling summary, 24h session entities, `remember` + embeddings, pending slot-fill.

Why it feels rigid:

- System prompt hard-routes images / PR / mail away from `agent_execute` (`prompt.go`).
- Specialists are in `AlwaysLoad` — the model can call `research_run` with a guessed topic and skip interview.
- Mail has no interview fields.
- Executions do not create board tasks.
- Dual paths (`agent_execute` vs direct specialist) fight each other.

## Evals first

Scripted Go tests (pattern in `server/internal/chatagent/agent_test.go`), plus launcher-level tests in `tasklaunch` / `platformtools`:

| ID | Probe | Assert |
|----|-------|--------|
| C1 | “research kubernetes cost optimisation” with 2+ projects | Missing `project`; after project → launcher creates task + research |
| C2 | “write an article about edge AI” | Interview `topic` if missing → launcher + article |
| C3 | “sync gmail” / “draft replies to support mail” | Launcher + mail path; never claims sent |
| C4 | “run the research agent” (thin prompt) | Asks what to research; does not invent a topic |
| C5 | Follow-up “do the same for staging” | Reuses `last_agent` / entities |
| C6 | “remember we only write about rust” then an article request | Memory / context includes the rust constraint |

Live (manual, after launcher): same probes against `qwen3-coder:30b` with out-of-band `GET /tasks`.

## Code changes

1. **All specialist execution goes through `tasklaunch`.** `runAgentExecute` and AlwaysLoad specialists (`research_run`, `article_generate`, `pentest_start`, mail sync) create/reuse a board task first.
   - **Project rule:** one project → use it; more than one → interview `project` unless session `last_project` is a clear reference.
2. **Narrow AlwaysLoad.** Prefer `agent_execute` for “run an agent”. Keep `image_generate` as a capability unless the user picked the Images bot.
3. **Prompt:** replace hard “do not pick an agent for X” with “start or run work via the Task Manager tools; interview missing slots; never invent topic/target/repo.”
4. **Memory:** on launch, inject `memory.Recall` hits and session entities into specialist `context`. Persist `last_task` / `last_project` from launcher results.
5. **Model:** keep `qwen3-coder:30b`. A/B `qwen3:30b-a3b` only if live evals still fail. Keep `llama3.1:8b` fallback. Skip `minicpm-v`, `all-minilm`, `llama3:latest`. Treat `muse-glimmer` as unknown until a tool-call smoke test.

## Acceptance

- Chat “research X” / “write about X” / “sync mail” creates a task on the chosen/only project and the card appears on the board.
- Scripted chat evals pass.
- No send-from-chat; Approve stays in the Mail UI.
