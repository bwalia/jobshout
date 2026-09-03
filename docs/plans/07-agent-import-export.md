# Agent Import & Export

How to use this file: **check a box when that item is done in code**, not when
it is only discussed. Walk Phase 1 top to bottom, then 2, then 3, then 4. Do
not skip a box because a nearby change “probably covers it” — open the surface
and confirm.

Related: `.claude/rules/agent-modules.md` (specialists register; platform stays
generic). Marketplace `POST /marketplace/{id}/import` is a **different**
in-cluster template copy — do not overload it.

---

## What we are implementing

Users export an agent from one JobShout org/environment and import the file
into another. The portable unit is the **org agent row plus named attachments**
(tools, skills, knowledge). Specialist **code** (Launch, schema, chat tools,
tab UI) does not travel; the destination already has it via `agentmodule`.

| Kind | Import behaviour |
|------|------------------|
| Custom agent | Always **create** a new row. IDs are not portable. Name clash → default `"{name} (imported)"`. |
| Builtin (`metadata.builtin`) | **Overlay** the destination’s seeded row (`FindBuiltin`). Never insert a second specialist. |
| Builtin missing on destination | **Block**. Do not silently import as a generic agent. |

First slice (Phase 1–2) is custom-agent round-trip + UI. Builtin overlay and
`Ready()` land in Phase 3.

---

## Product rules (lock these; do not invent new ones mid-PR)

| Situation | What happens |
|-----------|----------------|
| Export | One click. Server sanitizes and downloads JSON. Toast warnings if the file omitted secrets/connections. |
| Import happy path | Upload → **one preview screen** (validate + review + remap) → Confirm. Not a five-step wizard. |
| Preview | Mandatory in the UI. Confirm disabled while there are **errors**. |
| Secrets | Never in the file. Strip on **both** export and import. |
| Builtin overlay | Explicit confirm copy. No silent overwrite. Overlay **cannot be undone** in v1. |
| Custom create undo | Allowed only if the new agent has **zero executions** (`POST /agents/{id}/import/undo`). Overlay and specialists are not undoable. |
| Empty tool list | Preserved, but preview **warns** (executor runs with no tools). |
| `shell_command` (and other gated tools) | Preview call-out; **skip by default** unless the user opts in. |
| Marketplace | Unchanged. File import lives on Task Manager, not Marketplace. |
| Chat Agent | Out of scope (orchestrator, not an org agent). |
| New specialists | No import/export `switch`. Optional `Requirements` / `Ready` on `agentmodule.Module`. |

---

## Architecture

Platform-generic, same class as `GET /agent-schemas` and `POST /tasks/launch`.

```
Export:  Agent row + attachments  →  sanitizer  →  versioned JSON
Import:  package  →  preview (validate + deps + conflicts)  →  apply in one DB transaction
```

- **Package:** `server/internal/agentpack` — schema, sanitizer, preview, apply.
- **HTTP:** thin handlers next to existing `/agents` routes.
- **DB:** no new tables in v1. Use `audit_logs`.
- **Create path:** do **not** use today’s `AgentService.Create` as-is — it
  ignores `engine_type` / `engine_config`. Import writes the full row via the
  repository (or extend Create in the same PR as first import).
- **Org check:** export/import **must** match `agent.org_id` to the JWT org.
  Existing `GetByID` does not; do not copy that gap.

### Module hooks (Phase 3; no per-agent switches)

```go
Requirements []Requirement                    // static: gmail_oauth, strix, …
Ready        func(ctx, orgID) []Issue         // live check; never return secrets
```

Mail / career / pentest / PR review implement `Ready`. Others omit. Platform
calls `agentmodule.Lookup(builtin)`.

v1 does **not** export specialist extras (mail playbook, career profile,
GitHub). Users reconnect those on the destination.

---

## Package schema

File: `{slug}-YYYYMMDD.jobshout-agent.json`  
`Content-Type: application/vnd.jobshout.agent+json`  
`schema_version` integer; support current and previous; unknown fields ignored.

```json
{
  "kind": "jobshout.agent",
  "schema_version": 1,
  "exported_at": "2026-09-03T08:00:00Z",
  "source": { "agent_id": "<origin uuid>", "builtin": "article_writer" },
  "agent": {
    "name": "Article Writer",
    "role": "Content Writer",
    "description": "…",
    "system_prompt": "…",
    "model_provider": "ollama",
    "model_name": "qwen3-coder:30b",
    "engine_type": "go_native",
    "engine_config": { "structured_model": "…" },
    "builtin": "article_writer"
  },
  "tools": ["http_request", "web_search"],
  "skills": [
    { "slug": "cite-sources", "origin": "builtin" },
    { "slug": "my-voice", "origin": "org", "name": "…", "kind": "prompt",
      "config_json": { "prompt": "…" }, "config_override": {} }
  ],
  "knowledge": [
    { "filename": "style.md", "content": "…" }
  ],
  "warnings": []
}
```

**Include:** name, role, description, system prompt, model *preference*,
`engine_type`, sanitized `engine_config`, `builtin` only from metadata, tool
names, skills by slug (inline org-private defs), knowledge text.

**Never include:** API keys, OAuth tokens, `mail_connections.refresh_token_enc`,
`integrations.credentials`, `org_id`, `created_by`, `manager_id`, `avatar_url`,
status, performance score, executions, memory, goals, tasks, metrics, career
CVs/profile/pipeline, embeddings.

**Sanitize `engine_config`:** allowlist `structured_model`, `graph_definition`;
strip keys matching `(?i)(secret|token|password|api_key|credential|refresh)`.

**Limits:** 2 MB JSON, 50 knowledge files, 256 KB/file. Over → 413 / preview error.

Optional: `source.field_keys` from origin `agentschema` so preview can warn if
destination specialist fields drifted.

---

## APIs

```
GET  /api/v1/agents/{agentID}/export
POST /api/v1/agents/import/preview
POST /api/v1/agents/import
POST /api/v1/agents/{agentID}/import/undo
```

RBAC: `agents:read` export; `agents:create` create-import; `agents:update`
overlay; `agents:create` or `agents:delete` undo. Wire `RequirePermission` on
**these** routes even if older agent routes are still auth-only. Undo is
org-scoped and returns 409 if the imported agent has executions.

Preview returns `preview_id`, `mode` (`create` \| `overlay`), `issues[]`
(`error` \| `warning` \| `info`), and editable `bindings` (name, model remap,
`skip_tools`, skill creates).

Apply is **one transaction** (agent + tools + skills + knowledge). Embeddings
ingest **after** commit (best-effort, same as knowledge upload today).

---

## UI

Follow existing dialogs (`CreateAgentDialog`, marketplace `ImportAgentDialog`):
`role="dialog"`, Escape, block dismiss while pending, sonner toasts. **No**
new stepper. **No** dropzone library — hidden `<input type="file">` like Career
CV upload. Client-side JSON `kind` check like Skills config validation; server
is source of truth.

| Control | Where |
|---------|--------|
| **Import agent** | Task Manager header, next to **New agent** |
| **Export** | Task Manager agent header (next to Full profile) **and** `/agents/{id}` |

Import dialog states: idle → invalid file → preview loading → preview ready →
blocked (errors) → importing → success (toast + `?agent=<uuid>`) / failure
(stay on preview).

Builtin overlay copy:

> This organisation already has **{Agent}**. Import will update its prompt,
> model, and tools. Skills and knowledge in the file replace the current set;
> if the file has none, the current ones stay. Credentials are not in the file —
> reconnect on the agent tab if needed.

---

## Files (expected)

| Area | Files |
|------|--------|
| Pack | `server/internal/agentpack/` (schema, sanitize, preview, apply) |
| Module hooks | `server/internal/agentmodule/registry.go`; `Ready` on mail/career/pentester/prreview |
| Service | extend `agent_service.go` **or** `agentpack_service.go` composing agent/tool/skill/knowledge repos |
| HTTP | `agent_handler.go` or `agent_pack_handler.go`; routes in `cmd/server/main.go` |
| Audit | `audit_logs` via existing `AuditRepository.RecordAction` |
| API client | `web/nextjs/lib/api/agents.ts` |
| Hooks | `web/nextjs/lib/hooks/useAgents.ts` (export/import mutations) |
| UI | `ImportAgentPackageDialog.tsx` (do **not** reuse marketplace `ImportAgentDialog`); Export button on Task Manager + profile |
| Hosts | `TaskManagerPanel.tsx`, `app/(app)/agents/[id]/page.tsx` |
| Tests | `server/internal/agentpack/*_test.go`; handler/service tests; `web/nextjs/e2e/agents.spec.ts` |

Do **not** add cases to `TaskManagerPanel` BUILTINS, `tasklaunch/launch.go`,
`platformtools/execute.go`, or TypeScript `SCHEMAS`.

---

## Dependency handling (preview)

| Severity | Examples | User action |
|----------|----------|-------------|
| **Error** (blocks) | Wrong `kind` / too-new `schema_version`; builtin module missing; sanitizer found a secret-shaped field that could not be stripped safely | Fix package or upgrade destination |
| **Warning** | Model not in destination catalogue; tool name unknown; builtin skill slug missing; `Ready()` says Gmail disconnected; schema field keys drifted | Remap, skip, or connect later |
| **Info** | Overlay will change prompt; knowledge will be re-embedded; tools list empty | Acknowledge |

- **Model:** remap to Auto if enabled, else destination default; `ModelPicker`.
- **Tools:** match live registry by name. Missing → skip (default).
- **Skills:** builtin/community by slug; org-private **created** in destination (new IDs).
- **Knowledge:** insert files then existing ingest. If embedder off, store files anyway.

---

## Security (do not regress)

- Org isolation on every export/import load.
- Dual-side secret strip.
- Imported prompts/knowledge are untrusted (same as user-created agents).
- Audit `agent.export` / `agent.import` — hash or truncate huge prompts in `new_value`.
- JSON only in v1 (no zip bombs). Enforce Content-Length + decode limits.
- Knowledge may contain PII — disclose in preview; do not try to detect it.

---

## Out of scope (all phases unless a later plan says otherwise)

- Marketplace file packages / publishing an export to the catalog.
- Exporting Chat Agent, workflows, plugins, org-chart `manager_id`.
- Mail playbook / career profile / GitHub tokens inside the package (Phase 3+ extras hook, not v1).
- Overlay undo / snapshot table.
- Unique DB constraint on `(org_id, metadata->>'builtin')` (separate hardening PR).
- Wiring `RequirePermission` on **all** existing `/agents` REST routes (only the new pack routes are required here).
- Frontend role gating (none exists today).

---

## Phase 1 — package + export

Order is dependency-aware. Tick the matching **Master checklist** box when done.

### 1. Schema and sanitizer

- `agentpack` types for `kind`, `schema_version`, agent body, tools, skills, knowledge.
- `Pack` from an in-memory agent + attachments.
- `Sanitize`: strip secret-shaped keys; allowlist engine_config; drop ids/org/status/score.
- Size limit helpers.
- Unit tests: strip `api_key` / nested tokens; keep `structured_model`; omit org/user/ids.

### 2. Export API

- Load agent **in org**; missing and other-org both **404** (`agent not found`) so existence is not leaked.
- Load tools, skills (with org-private inline defs), knowledge files.
- `GET /agents/{id}/export` → JSON download (`Content-Disposition`).
- Warnings in file (e.g. “Gmail connection not included”) when builtin has `Requirements`.
  *(Static requirements may wait until Phase 3; until then a generic “credentials are never exported” warning is enough.)*
- Audit `agent.export`.

### 3. Export UI

- **Export** on Task Manager agent header and `/agents/{id}`.
- Immediate download; button “Exporting…”; error toast; warning toast if `warnings` non-empty.
- Over-limit: dialog “Export without knowledge?” (only if Phase 1 hits the cap in testing; otherwise 413 toast is enough).

### 4. Verify Phase 1

- Two orgs not required yet: export a custom agent, open the JSON, confirm no secrets and no `org_id`.
- Wrong-org id → 404 (same as missing).

---

## Phase 2 — import custom agents + UI

### 1. Preview + apply

- `POST /agents/import/preview` and `POST /agents/import`.
- Create mode only (custom). Name clash → suggested rename in bindings.
- Transaction: agent row (full engine fields) + tools + skills + knowledge.
- Knowledge ingest after commit.
- Model remap when provider/model missing.
- Dangerous tools skipped unless opted in.
- Empty tools → warning.
- Idempotent UI (disable double-submit). Optional `Idempotency-Key` can wait.
- Audit `agent.import`.

### 2. Import UI

- `ImportAgentPackageDialog`: hidden file input, JSON parse error inline, preview card, issue list, name field, Confirm.
- Task Manager **Import agent** next to **New agent**.
- Success: toast + select agent in rail (`?agent=<uuid>`).
- Blocked state: Confirm disabled.
- Do not reuse marketplace `ImportAgentDialog`.

### 3. Verify Phase 2

- Org A create custom agent (prompt, model, knowledge, skill, tools) → export → Org B import.
- New UUID; same prompt; new skill ids; knowledge present; no keys written from package.
- Browser: Task Manager import dialog happy path + invalid file.

---

## Phase 3 — builtin overlay + Ready()

### 1. Overlay policy

- `FindBuiltin` present → `mode: overlay`; confirm copy; `agents:update`.
- `FindBuiltin` absent / module not registered → error, no insert.
- Overlay updates prompt, model, engine_config, tools, skills, knowledge on the **existing** id.
- Tests: still exactly one row per builtin; missing specialist blocked.

### 2. Module Ready / Requirements

- Add fields on `agentmodule.Module`.
- Mail: Gmail disconnected warning.
- Career: no profile warning.
- Pentester: Strix not configured warning.
- PR reviewer: no GitHub integration warning.
- Platform iterates registry — no `if builtin ==`.

### 3. UI for overlay

- Preview shows diff (prompt/model/tools that will change).
- Copy states overlay cannot be undone from the dialog.
- Model remap + dangerous-tool opt-in on this path too.

### 4. Verify Phase 3

- Export Mail Agent from org A, overlay org B’s seeded Mail Agent.
- Export `career_ops` onto a stub registry without the module → blocked.

---

## Phase 4 — hardening

- RBAC on the three pack routes.
- Size limits enforced on preview/import (not only export).
- Undo-for-create when zero executions.
- Playwright: export download; import appears in rail.
- Security tests: package with `engine_config.openai_api_key` never lands in DB; `shell_command` skipped unless opted in.
- Optional: `source.field_keys` drift warning.

---

## Master checklist

Tick in this file when the work is in the tree and checked on the matching surface.

### Phase 1 — package + export

#### Schema / sanitizer

- [x] `server/internal/agentpack` package with versioned types (`kind`, `schema_version`)
- [x] `Pack` builds JSON from agent + tools + skills + knowledge
- [x] Sanitize strips secret-shaped `engine_config` keys
- [x] Sanitize keeps `structured_model` and `graph_definition`
- [x] Pack omits `id`, `org_id`, `created_by`, `manager_id`, status, performance score
- [x] Pack omits credentials, mail tokens, career PII, embeddings, executions, memory
- [x] Size helpers: 2 MB / 50 files / 256 KB per knowledge file
- [x] Unit tests for strip, allowlist, and omitted identity fields

#### Export API

- [x] `GET /api/v1/agents/{agentID}/export`
- [x] Org match required (404 other org and missing — no existence leak)
- [x] `Content-Disposition` filename `{slug}-YYYYMMDD.jobshout-agent.json`
- [x] Tools, skills (inline org-private), knowledge included
- [x] `warnings` array present (at least generic “credentials not exported”)
- [x] 413 when over size limits
- [x] Audit log `agent.export`
- [x] Routes registered in `main.go`

#### Export UI

- [x] Export control on Task Manager agent detail (next to Full profile)
- [x] Export control on `/agents/{id}` profile
- [x] Download works; loading disables the button
- [x] Error toast on 403/404/413
- [x] Warning toast when package `warnings` is non-empty

#### Verify Phase 1

- [x] Exported JSON has no secrets and no `org_id`
- [x] Cross-org export of another org’s id returns 404

### Phase 2 — import custom agents

#### Preview / apply

- [x] `POST /api/v1/agents/import/preview`
- [x] `POST /api/v1/agents/import`
- [x] Reject wrong `kind` / too-new `schema_version` (400)
- [x] Dual-side sanitize on import
- [x] Create mode writes **full** row including `engine_type` / `engine_config` (not incomplete `AgentService.Create`)
- [x] Name clash suggests `"{name} (imported)"` — never silent overwrite of a custom agent
- [x] One DB transaction: agent + tools + skills + knowledge; rollback on any error
- [x] Knowledge ingest after commit (best-effort)
- [x] Model remap when destination lacks the packaged model
- [x] Unknown tool names skipped by default; listed in preview
- [x] Empty tools list warns
- [x] Gated tools (`shell_command` at minimum) skipped unless opted in
- [x] Org-private skills created in destination with new IDs then enabled
- [x] Builtin/community skills matched by slug; missing → warning
- [x] Confirm disabled in UI while issues include `error`
- [x] Double-submit guarded in UI
- [x] Audit log `agent.import`
- [x] Import does **not** create a builtin row in this phase (custom only; unknown `builtin` → error or strip-to-custom **only if** we explicitly decide — default: **error** if `builtin` set)

#### Import UI

- [x] `ImportAgentPackageDialog` (new component; marketplace dialog untouched)
- [x] Hidden file input (no new dropzone library)
- [x] Invalid JSON / wrong `kind` shown inline (`role="alert"`)
- [x] Preview card: name, role, model, description
- [x] Issue list: errors red, warnings amber
- [x] Name field when clash
- [x] ModelPicker when remap needed
- [x] Escape / backdrop blocked while importing
- [x] Task Manager header **Import agent** next to **New agent**
- [x] Success toast + navigate to `?agent=<new-uuid>`
- [x] Failure keeps preview open; nothing partial in DB

#### Verify Phase 2

- [x] Round-trip custom agent between two orgs (prompt, model, knowledge, skill, tools)
- [x] Destination has new agent id; no packaged secrets in DB
- [x] Browser: invalid file, then happy-path import
- [x] Service tests: org isolation, transaction rollback

### Phase 3 — builtin overlay + Ready()

- [x] Overlay uses `FindBuiltin`; never `INSERT` with `metadata.builtin`
- [x] Missing module / missing seeded row → block (no generic fallback)
- [x] Overlay requires explicit confirm + `agents:update`
- [x] Overlay copy: cannot undo from the dialog
- [x] Preview diff for prompt / model / tools / skills / knowledge
- [x] `Requirements` + `Ready` on `agentmodule.Module`
- [x] Mail `Ready`: Gmail disconnected warning
- [x] Career `Ready`: no profile warning
- [x] Pentester `Ready`: Strix not configured warning
- [x] PR reviewer `Ready`: GitHub integration warning
- [x] No `if builtin == "mail"` (or similar) in `agentpack` / handlers
- [x] Test: overlay leaves a single builtin row
- [x] Test: unknown specialist blocked
- [x] Browser: overlay confirm copy shown for a seeded specialist

### Phase 4 — hardening

- [x] `RequirePermission` on export / preview / import
- [x] Size limits on preview and import (not only export)
- [x] Undo create: `POST /agents/{id}/import/undo` (org-scoped; 409 after executions; specialists blocked)
- [x] Overlay with empty packaged skills/knowledge leaves destination lists in place
- [x] No overlay undo (confirm we did not accidentally add a misleading Undo)
- [x] Playwright: export downloads; import shows in Task Manager rail
- [x] Security test: `openai_api_key` in package never persisted
- [x] Security test: `shell_command` not enabled unless opted in
- [x] Optional `source.field_keys` drift warning
- [x] This checklist updated as boxes are completed in code

---

## Risks (re-read before each phase)

1. **Duplicate builtins** — overlay must use `FindBuiltin`.
2. **Secret leakage** — `engine_config` and knowledge are the realistic holes.
3. **Version skew** — specialist code is not in the file; block unknown builtin.
4. **Empty tool list** — copying “no tools” disables the agent.
5. **Marketplace confusion** — file import stays on Task Manager.
6. **Incomplete `Create()`** — engine fields must be persisted.
7. **REST org gap** — pack endpoints must not copy `GetByID`’s missing org check.
8. **PII in knowledge** — disclose; cannot detect reliably.
