# Frontend route migration checklist

Old routes redirect to the chat-first panel IA (see `next.config.mjs`).
Functionality is preserved; only navigation structure changed.

| Old route | New location | Status |
|-----------|--------------|--------|
| `/` | `/chat` | done |
| `/dashboard` | `/panel/dashboard` | done (merged with metrics) |
| `/metrics` | `/panel/dashboard` | done (merged) |
| `/chat` | `/chat` (home) | done — history in left sidebar |
| `/sessions` | `/panel/sessions` | done — Session Manager panel (snapshots, context copy) |
| `/agent-board` | `/panel/task-board?view=agents` | done — Agents view in Task Board |
| `/tasks` | `/panel/task-board` | done — all-tasks kanban |
| `/task-manager` | `/panel/task-manager` | done |
| `/projects` | `/panel/task-manager` | done |
| `/projects/[id]` | `/panel/task-board?project=[id]` | done — per-project drag-and-drop board |
| `/agents` | `/panel/task-manager` | done |
| `/agents/[id]` | `/agents/[id]` (linked as "Full profile" from Task Manager) | kept — rich edit/knowledge/skills/metrics tabs |
| `/agents/[id]/knowledge` | `/agents/[id]/knowledge` | kept |
| `/agents/pentest` | `/panel/task-manager?agent=pentest` | done |
| `/agents/review` | `/panel/task-manager?agent=review` | done |
| `/articles` | `/panel/task-manager?agent=articles` | done |
| `/articles/[runId]` | `/articles/[runId]` (linked from Articles list) | kept |
| `/images` | `/panel/task-manager?agent=images` | done |
| `/scheduler` | `/panel/scheduler` | done |
| `/sprints` | `/panel/sprints` | done |
| `/workflows` | `/panel/workflows` | done |
| `/workflows/new`, `/workflows/[id]` | unchanged nested routes | kept (reachable from Workflows panel) |
| `/org-builder` | `/panel/org-builder` | done |
| `/marketplace` | `/panel/marketplace` | done |
| `/plugins` | `/panel/plugins-skills` | done (merged) |
| `/skills` | `/panel/plugins-skills` | done (merged) |
| `/llm-providers` | `/panel/llm-providers` | done |
| `/settings` | `/panel/settings` | done |

## Panel menu order

1. Chat → 2. Dashboard → 3. Task Board → 4. Task Manager → 5. Scheduler →
6. Sprints → 7. Sessions → 8. Workflows → 9. Org Builder → 10. Marketplace →
11. Plugins & Skills → 12. LLM Providers → 13. Settings
