# Task Manager — Plan

## Purpose

Let a person pick an agent, supply **that agent’s** inputs, assign a board task, run it, and see the result on the task board.

## Current behaviour

Create & run: pick agent → `getAgentInputSchema` → `AgentInputFields` → `POST /tasks` → `launchAgentForTask`.

Schemas exist for article, researcher, pentest, review, mail. Gaps:

- **Images:** rail entry, no schema, no launch implementation.
- **Mail:** six optional jargon fields; server chat schema empty.
- **Re-run:** `RunTaskDialog` resets to defaults; does not hydrate from the last launch.
- **Edit:** title/status only — no agent fields.
- **After launch:** article navigates to `/articles/{id}`; mail opens the inbox panel, not the new task; research has no run view.
- **PR `dry_run`:** UI default false, server/chat default true.

## Code changes (do not wait on agent evals)

### 1. Single server launcher

Package `server/internal/tasklaunch` and `POST /api/v1/tasks/launch`:

- Input: `agent_id`, `project_id` (optional when reusing `task_id`), `values` map, optional existing `task_id`.
- Creates the board task (`assigned_agent_id`, title/description from the same rules as `titleFrom` / `descriptionFrom`).
- Persists `launch_values` (and run ids) in `tasks.metadata` (column already exists; wire it through the Go model + repository).
- Dispatches the specialist with `task_id` where the API supports it.
- Returns `{ task, kind, run_id, sync_queued, brief, … }`.

Frontend `launch.ts` becomes a thin client of this endpoint (one source of truth with chat).

### 2. Per-agent input flow

- Keep specialist schemas; when no agent selected, show only “Choose an agent”.
- **Images:** add a schema (`prompt` required) that creates a task + image generate.
- Hydrate Run dialog from `task.metadata.launch_values`.
- Align `dry_run` default to **true** in `input-schemas.ts`.

### 3. Board visibility after run

- Always stay in Task Manager: `?project=&task=&run=` (or `agent=mail&thread=`).
- Article: do not hard-navigate to `/articles/{id}`; show the task + link to the article.
- Mail: task `in_progress` until a draft is ready or sync reports empty.
- Research: write the brief onto the task (already); keep the user on the task.

### 4. E2E

New `web/nextjs/e2e/task-manager-launch.spec.ts`: for each specialist, select agent → required fields appear. Mock or skip live GPU/Gmail.

## Acceptance

- Selecting Mail / Article / Research / Images / generic always shows that agent’s fields.
- Create & run always creates a visible board task and starts the executor (or a clear connect/error state).
- Re-run shows the last inputs.
