# Task history, Run from the board, and honest progress

How to use this file: **check a box when that item is done in code**, not when it is only discussed. Implementation should walk the Phase 1 list top to bottom, then Phase 2. Do not skip a box because a nearby change “probably covers it” — open the surface and confirm.

Related: [04-task-manager.md](04-task-manager.md) (launch + stay on the task). This plan is the UX/progress follow-up.

---

## What we are implementing first

The first slice is the four user-facing gaps that make the board and Task Manager lie or feel dead:

1. Clicking a task on the **Task Board** must let you **Run** it and **Show History**.
2. Every task in **Task Manager** must show **history** and a real **completed-at** time.
3. A **Done** task must not look like it still needs to be run.
4. Card / list **progress** must follow the actual run (queued → running → completed/failed).

Everything else stays on the Phase 2 checklist so it is not forgotten.

---

## Product rules (lock these; do not invent new ones mid-PR)

| Situation | What the user sees |
|---|---|
| Task is `done` | Primary action is **Show History**. No primary **Run**. Optional muted **Re-run** only inside History. |
| Task is not `done` | **Run** is available on the open-task surface (board modal and Task Manager detail). |
| User clicks a board card | Same slide-over as today, plus **Run** (if allowed) and **Show History**. |
| User clicks **Show History** | One shared history panel: status timeline, every run (generic + specialist pointer), outputs, errors, completed-at. |
| Generic run starts | Board card moves to **In Progress**; list/card shows a live run chip. |
| Generic run completes | Board card moves to **Done**; `completed_at` is set; Run is demoted. |
| Generic run fails | Card stays **In Progress** (no `failed` column). Card/list shows a failed chip. Run stays available. |
| Specialist with no `task_runs` row | History still lists the launch (kind, link, time) instead of “no runs”. |
| Done task is edited later | `completed_at` does **not** change. `updated_at` may. |

Re-run of a Done task (from History) is allowed: it moves the task back to **In Progress**, clears `completed_at`, and appends a new history/run row.

---

## Design

### Shared UI (one implementation, many hosts)

Add two small pieces under `web/nextjs/components/task-manager/` and reuse them everywhere. Do not fork a third history UI in the Kanban modal.

- `TaskActions.tsx` — **Run** / **Re-run** / **Show History** visibility from `task.status` + latest run.
- `TaskHistoryPanel.tsx` — slide-over or dialog. Header shows title, status, **Completed** timestamp (or “Not completed”). Body:
  1. Status timeline (`GET /tasks/{id}/history`)
  2. Existing `TaskRunView` for generic runs
  3. Specialist launch row when `metadata.launch_kind` is set and there is no matching generic run
- `TaskProgressChip.tsx` — compact “Queued / Running / Completed {time} / Failed” from `last_run_*` fields, used on list rows and board cards.

`TaskDetailModal` gains Run + Show History in the footer/header. It must not close on Save when the user only wanted to run.

`RunTaskDialog` stays as the run console. Hosts pass `onLaunched` / `onSpecialistLaunched` and then open History focused on that run.

### Backend: timestamps and history that actually exist

`task_status_history` is already in the schema and **never written**. Metrics already read it. First slice makes it real.

Migration `000037` (idempotent `IF NOT EXISTS` / `ADD COLUMN IF NOT EXISTS`, same rule as 000027):

- `tasks.completed_at TIMESTAMPTZ NULL`
- Backfill: for current `status = 'done'`, set `completed_at = updated_at` once (best effort; better than blank)

On every status write (`TransitionStatus` and `Reorder` when status changes):

1. Insert `task_status_history` (`old_status`, `new_status`, `changed_by` if we have a user).
2. If `new_status = 'done'`, set `completed_at = NOW()` only when it is currently null (first completion wins; a later re-done after re-run sets a new time because we cleared it on leave).
3. If leaving `done`, set `completed_at = NULL`.

`GET /api/v1/tasks/{taskID}/history` returns newest-first:

```json
{
  "completed_at": "...",
  "events": [
    { "id": "...", "kind": "status", "old_status": "in_progress", "new_status": "done", "changed_at": "..." },
    { "id": "...", "kind": "run", "run_id": "...", "status": "completed", "agent_id": "...", "created_at": "...", "completed_at": "..." }
  ]
}
```

Runs can be inlined from `task_runs` plus a synthetic event from `metadata.launch_kind` / `run_id` when there is no generic row. Keep the existing `GET /tasks/{id}/runs` for `TaskRunView` polling.

### Backend: list payload so cards do not N+1

Extend the task JSON (list + get) with:

- `completed_at`
- `last_run_status` (`queued` \| `running` \| `completed` \| `failed` \| null)
- `last_run_at` (run `completed_at` or `created_at`)
- `last_run_id`

Fill with a `LEFT JOIN LATERAL` on the latest `task_runs` row in list/get scans. No extra request per card.

### Backend: run updates the board card

In `TaskRunService`:

- `CreateRun`: after insert, `Transition` the task to `in_progress` (ignore if already there).
- `finalize` success: `Transition` to `done`.
- `finalize` failure: do not mark done; invalidate is enough once last_run fields refresh.

Also invalidate: `useCreateTaskRun` and run polling must `invalidateQueries({ queryKey: taskKeys.all })` when a run enters a terminal state so the board moves without a manual refresh.

### Backend: transition response (blocks status dropdown snap-back)

`PATCH /tasks/{id}/transition` and `PUT /tasks/{id}/position` must return the **full task** (including `completed_at` and last-run fields), not `{ "status": "done" }`.

`useTransitionTask` / `useReorderTask` already assume a `Task`. After the handler fix they will invalidate the real `id` / `project_id`. Also invalidate `taskKeys.all` so the all-projects board updates.

`Update` must treat `assigned_agent_id: null` as unassign (Phase 2 box, but do it in the same service pass if you are already in `task_service.go`).

---

## Phase 1 — implement first

Order is dependency-aware. Check the matching box in **Master checklist** when done.

### 1. Data and APIs

- Migration `000037_task_completed_at.up.sql` + `.down.sql`.
- Write `task_status_history` in `TransitionStatus` and status-changing `Reorder`.
- Set / clear `tasks.completed_at`.
- `ListByProject` / `ListByOrg` / `FindByID` select the new columns + last-run lateral join.
- `Transition` / `Reorder` handlers return the full task.
- `GET /tasks/{taskID}/history`.
- `TaskRunService.CreateRun` / `finalize` move the board status.
- Tests: transition writes history + completed_at; run complete → task done; history endpoint shape.

### 2. Shared frontend

- Types: `completed_at`, `last_run_*` on `Task`; history types + `getTaskHistory`.
- `TaskActions`, `TaskHistoryPanel`, `TaskProgressChip`.
- `TaskRunView`: completed uses a distinct color from running; prefer `completed_at` in the list; empty copy says **Run** (same as the button).
- `useTransitionTask` / run hooks invalidate `taskKeys.all`.
- `useTaskRuns` refetch still drives the chip via task list invalidation when a run goes terminal.

### 3. Task Manager

- Each project task row: status, **completed-at** (or last updated if not done), `TaskProgressChip`, **Show History**.
- Detail header: History always; **Run** only when not done.
- Agent “Assigned tasks”: same row treatment; clicking the row opens History for **that** task; remove the `tasks[0]`-only runs block (or retarget it to the selected task).
- Agent header **Run task** must not silently run `tasks[0]` if that task is done — prefer first non-done assigned task, else open create.

### 4. Task Board + Kanban modal

- `TaskDetailModal`: **Show History**; **Run** when not done (opens `RunTaskDialog`).
- After a run from the modal, open History on that run; do not only close the editor.
- All-projects cards: `TaskProgressChip` + completed-at when done.
- Kanban cards: same chip (small) so In Progress vs “run failed” is visible.

### 5. Verify

- Browser: Task Manager row → History; Done task has no primary Run; completed-at visible.
- Browser: Task Board click → Run + History; after run, card moves to In Progress then Done.
- Playwright: extend `projects-and-tasks.spec.ts` / `task-manager-launch.spec.ts` for History button and Done hiding Run.

---

## Phase 2 — do not forget (after Phase 1)

These were in the audit. They are not required to ship the first slice, but each has a checkbox below.

- Agent board cards clickable + include `task_runs` in activity. **Done.**
- Specialist async completion (article / pentest / review stay `in_progress` today). **Done** (success → done; failure stays in progress).
- Shared status-dot palette (kill Kanban `COLUMN_DOT` duplicate Done badge). **Done.**
- Unify all-projects vs per-project board (URL `?task=`, one modal, drag story). **Done.**
- Task Manager `?task=` / `?run=` on click, not only after launch. **Done.**
- Unassign agent from the modal (`null` must clear). **Done.**
- Lists that silently stop at 200 tasks. **Done** (cap 200 + page walk + “Showing X of Y”).

---

## Files (expected)

| Area | Files |
|---|---|
| Migration | `000037_task_completed_at` (+ down), `000038_review_run_task_id` (+ down) |
| Models | `server/internal/model/task.go` |
| Repo | `server/internal/repository/task_repository.go` |
| Service | `server/internal/service/task_service.go`, `task_run_service.go` |
| HTTP | `server/internal/handler/task_handler.go`, `server/cmd/server/main.go` |
| Types / API | `web/nextjs/lib/types/project.ts`, `lib/api/tasks.ts`, `lib/hooks/useTasks.ts`, `lib/hooks/useTaskRuns.ts` |
| Shared UI | `web/nextjs/components/task-manager/TaskActions.tsx`, `TaskHistoryPanel.tsx`, `TaskProgressChip.tsx`, `TaskRunView.tsx` |
| Hosts | `TaskDetailModal.tsx`, `TaskManagerPanel.tsx`, `TaskCard.tsx`, `TaskBoardPanel.tsx`, `app/(app)/task-manager/page.tsx` |
| Tests | `task_run_service_test.go` (or new task history test), `e2e/projects-and-tasks.spec.ts` |

---

## Master checklist

Tick in this file when the work is in the tree and checked on the matching surface.

### Phase 1 — first slice

#### Data / API

- [x] `tasks.completed_at` column + backfill for existing done tasks
- [x] `task_status_history` row written on transition
- [x] `task_status_history` row written when reorder changes status
- [x] Leaving `done` clears `completed_at`; entering `done` sets it
- [x] Task list/get JSON includes `completed_at`, `last_run_status`, `last_run_at`, `last_run_id`
- [x] `PATCH /tasks/{id}/transition` returns the full task
- [x] `PUT /tasks/{id}/position` returns the full task
- [x] `GET /tasks/{id}/history` returns status events + runs (+ specialist pointer)
- [x] Generic `CreateRun` moves the task to `in_progress`
- [x] Successful generic run moves the task to `done`
- [x] Failed generic run does **not** mark the task done
- [x] Server tests cover history write, completed_at, and run → board status

#### Shared UI

- [x] `TaskActions` hides primary Run when `status === "done"`
- [x] `TaskHistoryPanel` shows status timeline, runs, specialist link, completed-at
- [x] Show History works with zero generic runs (specialist-only or never run)
- [x] `TaskProgressChip` on list/card: queued / running / completed / failed
- [x] `TaskRunView` completed style is not the same as running
- [x] `TaskRunView` timestamps use `completed_at` when present
- [x] Empty runs copy says **Run**, not **Run now**
- [x] Task query invalidation uses `taskKeys.all` after transition and terminal runs

#### Task Manager

- [x] Each project task row shows completed-at (or “Updated …” if not done)
- [x] Each project task row has **Show History**
- [x] Project task detail: History always; Run hidden when done
- [x] Agent assigned-task row has **Show History** and completed-at
- [x] Clicking an assigned task opens **that** task’s history (not `tasks[0]`)
- [x] Agent header Run does not target a done `tasks[0]`
- [x] Legacy `/task-manager` page matches the same Run / History rules

#### Task Board / Kanban

- [x] Board / Kanban `TaskDetailModal` has **Show History**
- [x] Board / Kanban `TaskDetailModal` has **Run** when the task is not done
- [x] Running from the modal does not require leaving the board
- [x] All-projects cards show `TaskProgressChip` and completed-at when done
- [x] Per-project Kanban cards show the same progress chip
- [x] After a generic run, the card moves In Progress → Done without a full page reload

#### Verify Phase 1

- [ ] Browser: Task Manager — Done task, History, no primary Run, timestamp
- [ ] Browser: Task Board all-projects — click, Run, History
- [ ] Browser: Task Board project Kanban — click, Run, History, card moves
- [x] Playwright covers History visible and Run absent on a done task

### Phase 2 — remaining audit items

- [x] Agent board card opens the current job / task (clickable)
- [x] Agent board activity includes generic `task_runs` (not idle while a run is live)
- [x] Article / pentest / PR review mark the board task done (or failed) when the specialist run finishes
- [x] Kanban column dots use `STATUS_DOT`; remove the extra Done chip
- [x] All-projects board and project Kanban share one open-task path (no double modal)
- [x] Clicking a Kanban card writes `?task=` on the Task Board URL
- [x] Task Manager click writes `?project=&task=` (and keeps `run` when relevant)
- [x] Unassign in `TaskDetailModal` persists (`assigned_agent_id: null` clears)
- [x] Lists over 200 tasks are not silently truncated (pager or higher cap + total shown)

---

## Out of scope for both phases

- New Kanban column named Failed.
- Rewriting the agent-board column model (planning / publishing) beyond feeding `task_runs` into existing columns.
- Chat-launcher changes (still [05-chatbot.md](05-chatbot.md)).
