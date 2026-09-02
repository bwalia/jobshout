# Agent modules

A new builtin agent is **its own package**. Do not edit the platform to special-case it.

The agent owns: seed (name, role, prompt, `metadata.builtin`), launch schema, `Launch`, optional extra chat tools, a one-line chat hint, optional tab UI.

Register on three surfaces only:

1. **Task Manager** — rail tab from the registry. No new `BUILTINS` row or `selection.id === "…"` in `TaskManagerPanel.tsx`.
2. **Chat** — `agent_execute` + schema interview. Extra tools live with the agent. No new bullet in `chatagent/prompt.go`, no new `if` in `platformtools/execute.go`.
3. **That agent’s tab** — render the agent’s input schema (same fields as New task / chat). Custom product UI (`*AgentClient`) mounts under the form and lives with the agent.

Platform that must stay generic: `AgentInputFields`, `POST /tasks/launch`, `agent_execute`, board tasks, seed-on-register (iterate the registry).

Do not duplicate launch fields in Go and TypeScript. One schema (`agentschema` / `GET /api/v1/agent-schemas`); the web consumes it.

Existing switches (`tasklaunch/launch.go`, `input-schemas.ts`, `TaskManagerPanel` `BUILTINS`) are debt. New agents must not add cases.

Cursor copy of this rule: `.cursor/rules/agent-modules.mdc`.
