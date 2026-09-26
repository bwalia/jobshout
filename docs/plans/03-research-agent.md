# Research Agent — Plan

## Purpose

Produce a cited `research.Brief` (summary + findings with quotes + sources) that other agents and the board can trust.

## Current behaviour

`Agent.Research` in `server/internal/research/agent.go`: plan → search → select → read → extract → verify → synthesise. Pinned `Request.URLs` skips plan/search (`researchPinned`).

Consumers: Article (`writeOne`), Mail, Task Manager `POST /research` (sync, 180s), chat `research_run`.

Task Manager marks the board task `done` and writes the brief into the description. No `TaskRun` row — “Runs” is empty.

Believed correct. **No product rewrite unless an eval fails.**

## Evals (confirmation set)

File: `server/internal/research/eval_test.go` using existing backend fakes.

| ID | Scenario | Assert |
|----|----------|--------|
| R1 | Open-web “Kubernetes cost optimisation” | `Brief.IsUsable()` (findings + sources); quotes verify against fetched text |
| R2 | Pinned URLs | Search not called; only those URLs are read |
| R3 | Thin / unreadable URL | Warnings populated; does not panic; `IsUsable()` false or recovery documented |
| R4 | Mail-shaped request (subject + body in `Context`) | Usable briefing |

R2 is already largely covered by `TestResearch_PinnedURLsSkipPlanAndSearch` in `agent_test.go`; the eval file should call the same helpers so the four cases live in one place.

## After Task Manager / chat launcher

Attach `task_id` when launching from the board or chat. Keep writing the brief onto the task description (already good UX).

## Acceptance

- The four evals pass on the current agent.
- Failures become tickets before any research rewrite.
