# JobShout — Agent module contract (next PR)

> Paste everything below the line into a fresh agent. New branch
> `feat/agent-module-contract`. Do not mix Career product work into this PR.
> Rules to keep afterwards: `.claude/rules/agent-modules.md` (Claude Code)
> and `.cursor/rules/agent-modules.mdc` (Cursor).

---

## Goal

Adding a JobShout specialist must not mean editing the platform. The agent is
its own code. You **register** it on three surfaces. After this PR, agent N+1
does not touch `TaskManagerPanel` switches, `tasklaunch` cases,
`chatagent/prompt.go`, or `input-schemas.ts`.

## Three wire-ups (the whole contract)

1. **Task Manager** — rail tab from a registry (`label`, `icon`, `builtin`). Clicking the tab shows **that agent’s input schema** (same fields as New task / chat). Optional `*AgentClient` mounts under the form; it is not a new `if` in the panel.
2. **Chat** — `agent_execute` interviews from the same schema and calls the same `Launch`. Extra tools (`career_scan`, `mail_sync`, …) register with the agent. A one-line chat hint is data on the module, injected into the system prompt — do not hand-edit `prompt.go` per agent.
3. **Per-agent tab** — the tab **is** the input surface. New task / Run reuse the same schema. No one-off dialogs per agent.

## Platform that stays generic (do not special-case)

- Form renderer: `web/nextjs/components/task-manager/AgentInputFields.tsx`
- HTTP: `POST /api/v1/tasks/launch`
- Chat: `agent_execute` in `server/internal/platformtools/`
- Board tasks, seed-on-register (iterate the registry)

## Source of truth

One launch schema. Server: `server/internal/agentschema` + `GET /api/v1/agent-schemas` (already exists). Web must consume that API (or a generated client). Delete the duplicate `SCHEMAS` map in `web/nextjs/lib/agents/input-schemas.ts` once the web reads the API. Keep `parity_test.go` until the TS copy is gone.

## Suggested shape

Go: a `Specialist` (or `AgentModule`) with builtin key, display name/role, seed, schema, `Launch(ctx, Request) (Result, error)`, optional chat tools, chat hint, prompt→field mapping.

Web: a module with `builtin`, `label`, `icon`, optional `Tab` component. Rail and tab mount from the list. No `BUILTINS` array and no `selection.id === "career"` blocks in `TaskManagerPanel.tsx`.

`tasklaunch.Service` becomes a map of builtin → `Launch`, not a struct field per agent. `auth_service.seedBuiltinAgents` iterates the same registry.

## How to land it (do not big-bang)

1. Add the registry and the generic TM tab (schema form + this agent’s tasks).
2. Point Career Agent through it first (already named **Career Agent**, builtin `career_ops`). Tab must show job URL / JD / mode from the schema; `CareerAgentClient` sits under that form.
3. Migrate Research, then Mail, Articles, Images, Pentest, PR Reviewer — one specialist per commit if needed. Each migration **removes** a `case` / `BUILTINS` row, it does not add one.
4. Chat: generate the specialist bullets from module hints; delete per-agent `switch` in `execute.go` (prompt→field lives on the module).
5. Tests: parity (schema keys), seed still creates builtins, TM tab renders Career fields, `agent_execute` still interviews Career, existing Mail/Articles e2e still pass.

## Out of scope

- Career eval quality, A–H blocks, new career endpoints
- Renaming APIs (`/career/*`, `career_ops`)
- New k8s apps or extra Helm charts

## Done when

- A new specialist can be added by writing its package + one register call
- `TaskManagerPanel.tsx` has no per-agent `if`
- Launch fields are not copied in TypeScript
- `.claude/rules/agent-modules.md` and `.cursor/rules/agent-modules.mdc` still match the code
