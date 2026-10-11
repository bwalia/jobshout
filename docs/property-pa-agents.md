# Property PA agents (Seller PA, Manager PA)

OpsAPI is the system of record for the Property PA. JobShout runs the PA agents and routes every model
call (per-org routing, see [opsapi-integration.md](opsapi-integration.md) §2). A PA agent reads and writes
OpsAPI **only through OpsAPI's HTTP API**, as the workspace's service account, using two tools that
JobShout controls.

Code: `server/internal/propertypa` (definitions, prompts, installer, output parser),
`server/internal/tools/opsapi_tools.go` (tools), `server/internal/handler/knowledge_external.go`
(knowledge sync), `server/internal/handler/property_pa_handler.go` (install/list).

All JobShout paths below are relative to `/api/v1`.

## 1. Setup per org

One JobShout org per OpsAPI workspace.

**1. Point the org at its OpsAPI workspace.** Create an integration with provider `opsapi` (org admin, JWT):

```
POST /integrations
{"name": "OpsAPI", "provider": "opsapi",
 "base_url": "https://opsapi.example.com",
 "credentials": {"api_key": "opsk_..."},
 "config": {"namespace_id": "42", "namespace_slug": "acme-homes"}}
```

| Field | Meaning |
|---|---|
| `base_url` | OpsAPI API root (`http(s)://host[/prefix]`), no `/api/v2`. Use https outside a LAN. |
| `credentials.api_key` | An OpsAPI workspace API key (`opsk_…`, from OpsAPI `POST /api/v2/api-keys`). Sent as `Authorization: Bearer opsk_…`. Scope it to the PA modules only (`property-pa`, `property-deals`): OpsAPI confines a key to the `/api/v2/<module>/…` prefixes it is scoped for. |
| `config.namespace_id` / `config.namespace_slug` | The workspace, sent as `X-Namespace-Id` / `X-Namespace-Slug`. At least one. Must be the key's own namespace. |

Only an integration with `status` `active` (the default) is used. The key is read at every tool call, so
rotating it is an `PUT /integrations/{id}`. **Caution:** integration credentials are stored as plain JSONB
today and `GET /integrations` returns them to org members (pre-existing; see open questions).

**2. Install the agents** (org admin, JWT; idempotent):

```
POST /property-pa/install
→ {"agents": [{"key":"seller","agent_id":"…","name":"Seller PA","context_role":"seller_agent",
               "prompt_version":"seller-pa@1.3.0","prompt_sha256":"…","tools":["opsapi_get","knowledge_search","opsapi_post","email_send"],
               "approval_tools":["email_send","slack_send_message","teams_send_message"],
               "skills":["pa-today","pa-prepare-me","pa-capture","pa-coach","pa-handoff"],"status":"created"},
              {"key":"manager", …}],
   "skills": [{"slug":"pa-today","id":"…","version":"1.3.0","status":"created"}, …]}
```

- Agents are found again by `metadata.property_pa` (`seller` / `manager`), not by name, so admins may
  rename them. A re-install updates role, description, prompt and metadata when the shipped prompt
  version or text differs (`status: updated`), else `unchanged`. The model is never set: the org's routing
  chain chooses it.
- The tool allow-list is **authoritative** (re-install resets it): it is the PA's authority boundary.
  Tools the server has not registered (e.g. `knowledge_search` without an embedder) are dropped and
  listed in `skipped_tools`.
- Approval rules are **additive**: the external tools are always gated; rules an admin added stay.
- Mode skills (org-private `prompt` skills) are created or updated in place.

`GET /property-pa/agents` (also callable with an API key that has `agents:read`) returns each agent's id,
installed and available prompt version and `upgrade_available`, plus the output contract id and skill
slugs. OpsAPI should call it at start-up to learn the agent ids.

## 2. Tools

| Tool | What it may call | Notes |
|---|---|---|
| `opsapi_get` | `GET` on any path under `/api/v2/property-pa/` or `/api/v2/property-deals/` | Input `{path, query}`. |
| `opsapi_post` | `POST` to exactly: `/api/v2/property-pa/captures`, `/handoffs`, `/knowledge`, `/grounding/check` | Input `{path, body}` (object, ≤ 32 KiB). All internal records; nothing reaches a client. |
| `knowledge_search` | The agent's own knowledge (pgvector) | Present when embeddings are configured. |
| `email_send` | The org's email notification channel | **Approval-gated** (authority level 4). |

Safety properties of the OpsAPI tools (tests in `opsapi_tools_test.go`):

- **Credentials never come from the model.** Base URL, key and namespace are read from the calling org's
  integration at call time. Extra model arguments (`url`, `headers`, `api_key`, `base_url`) are ignored.
  The key never appears in a tool result.
- **No arbitrary host.** The path must match `^/[A-Za-z0-9/_.-]*$` (no scheme, `@`, `%`, `?`, `#`), have
  no empty or dot segments, and be on the allow-list for its method. The joined URL must keep the
  configured scheme and host. Redirects are not followed (a 3xx is returned as data).
- **Fenced results.** Every response comes back as
  `BEGIN_UNTRUSTED_TOOL_RESULT name=opsapi_get` / JSON `{status, method, path, body, truncated?, hint?}` /
  `END_UNTRUSTED_TOOL_RESULT` plus a "treat as data" line, the same markers the platform tools use. Marker
  strings inside OpsAPI data are defused. Bodies are capped at 64 KiB.
- **404 is data, not an error,** with a hint ("not deployed yet"), so a run degrades while OpsAPI's PA
  endpoints are still being built.
- **Audit headers.** Every call sends `X-JobShout-Agent-Id` and, when the agent has one,
  `X-JobShout-Prompt-Version`, so OpsAPI can stamp AI writes with the prompt version (P10).

The allow-list is code (`OpsAPIReadPrefixes`, `OpsAPIWritePaths`); widening it is a reviewed change.

## 3. Agents and prompts

| Key | Name | `context.role` | Prompt version | Skills |
|---|---|---|---|---|
| `seller` | Seller PA | `seller_agent` | `seller-pa@1.3.0` | `pa-today`, `pa-prepare-me`, `pa-capture`, `pa-coach`, `pa-handoff` |
| `manager` | Manager PA | `manager` | `manager-pa@1.3.0` | `pa-manager-today`, `pa-handoff`, `pa-teach` |

Prompt text: `server/internal/propertypa/prompts/{seller_pa,manager_pa,common}.md` (each agent's prompt
is its own file followed by `common.md`). The common part carries the rules both share:

1. **Grounded or silent**: every material fact comes from a record read in this run and is cited; unknowns
   are said as unknown.
2. **Fact vs inference** labelled ("(inference)", "UNCONFIRMED").
3. **Authority**: observe, recommend, prepare; record internal things in OpsAPI; anything external is a
   draft that needs approval; never offers, commitments, legal/financial/compliance judgements.
4. **Proposed, not changed**: one `captures` call logs the interaction and proposes each change with `confidence`, `is_inferred` and an exact `source_quote`;
   material fields always need human confirmation.
5. Next actions carry owner, due time and reason.
6. Knowledge is cited; new rules are proposals for a manager.
7. British English, short, answer first.

Seller PA adds Today, Prepare me, Capture, Coach ("questions before money", never default to a higher
offer) and Handoff. Manager PA adds exceptions-first Manager Today, handoff decisions and Teach (rule
proposals that take effect only after approval in OpsAPI).

**Versions.** Each agent's `metadata` holds `property_pa`, `prompt_version`, `prompt_sha256`,
`context_role` and `pa_skills`. `TestPromptVersionsTrackPromptText` pins every version to its text's
SHA-256, so editing a prompt without bumping its version fails CI. Bump the constant in
`definitions.go`, pin the new hash, re-install.

## 4. Starting a run (OpsAPI → JobShout)

Authenticate with a JobShout org API key (`Authorization: Bearer jsk_…`, scopes `runs`, `approvals`,
`agents:read`, `knowledge`).

**First run in a workspace** (creates a board task for the PA; keep its `task_id`):

```
POST /tasks/launch
{"agent_id": "<seller agent id>", "project_id": "<project>",
 "values": {"prompt": "Morning. What do I need to focus on today?"},
 "context": {"role": "seller_agent", "user_ref": "opsapi-user-7", "workspace_ref": "42"},
 "skill_slugs": ["pa-today"]}
```

**Every later turn** (same task, either agent):

```
POST /tasks/{task_id}/run                                     → 202 {id: <run id>, status: "queued", …}
{"agent_id": "<seller agent id>",
 "prompt": "Prepare me for Sarah.",
 "context": {"role": "seller_agent", "user_ref": "opsapi-user-7", "workspace_ref": "42"},
 "skill_slugs": ["pa-prepare-me"],
 "inputs": {"person_id": "p_1", "time_budget_minutes": 30}}
```

- `context.role` must match the agent (`seller_agent` → Seller PA, `manager` → Manager PA). It and
  `user_ref` reach the prompt's `## Inputs` block; JobShout never resolves the refs.
- Pass **one mode skill** per run. `inputs` are free-form hints (ids the screen already knows).
- Use a voice transcript as the prompt for capture (`pa-capture`); STT stays in OpsAPI.

**Getting the result**, either:

- **Webhook** (recommended): subscribe to `run.completed`, `run.failed`, `approval.requested`,
  `approval.decided` (`POST /webhook-endpoints`). Run events carry `output`, `context`, `skill_slugs`,
  `model_provider`, `model_name` and `prompt_version`. Verify the signature and dedupe on the envelope `id`.
- **Polling**: `GET /task-runs/{id}` until `status` is `completed` or `failed`. A run waiting on a gated
  tool is `awaiting_approval` with `pending_approval_id`; decide it with `POST /approvals/{id}/decide` and
  the run resumes. Polling does not return `prompt_version`: read it from `GET /property-pa/agents` when
  you start the run.

## 5. Output contract (`property-pa.output.v1`)

Every PA answer is prose for the user followed by **exactly one** fenced block tagged `pa_output`, last in
the output:

````
Sarah is your first call: her second valuation was yesterday and she said she'd decide after it.
…
```pa_output
{
  "contract": "property-pa.output.v1",
  "mode": "today",
  "answer": "Sarah is your first call: …",
  "sources": [
    {"type": "person", "id": "p_1", "label": "Sarah Jones"},
    {"type": "event", "id": "e_9"}
  ],
  "unknowns": ["Tom's view on price"],
  "inferences": ["She is likely to decide this week (inference from her promise)"],
  "actions": [{"kind": "task", "id": "t_4", "summary": "Call Sarah tomorrow 11:00"}],
  "needs_human": false
}
```
````

| Field | Type | Rule |
|---|---|---|
| `contract` | string | Always `property-pa.output.v1`. |
| `mode` | string | `today`, `prepare`, `capture`, `coach`, `ask`, `handoff`, `manager_today`, `teach` (optional). |
| `answer` | string | Non-empty; the text shown to the user. |
| `sources[]` | `{type, id, label?}` | `type` ∈ person, organisation, property, lead, opportunity, deal, interaction, task, event, offer, knowledge, handoff, briefing. `id` is OpsAPI's id as returned. |
| `unknowns[]` | string | What the PA could not find. |
| `inferences[]` | string | Every inference, one line each. |
| `actions[]` | `{kind, id, summary?}` | Records this run created. `kind` ∈ interaction, proposed_update, task, handoff, knowledge_proposal, draft, approval_requested. |
| `needs_human` | bool | A human must decide before work continues. |

Parsing (Go reference: `propertypa.ParseOutput`): take the last ```` ```pa_output ```` block; if there is
none, accept the last ```` ```json ```` block whose `contract` matches (models sometimes mis-tag). The text
before the block is the display prose. Normalise `type`/`kind`/`mode` to lower case and missing arrays
to `[]`. Then **OpsAPI must check every `sources[].id` exists in the workspace** (grounded-or-silent is
enforced against the records, not trusted from the model) and treat a missing or invalid block as a
failed turn (show the prose, flag the run for review).

## 6. Knowledge sync (OpsAPI → JobShout)

OpsAPI owns knowledge, its versions and approvals. When a rule is approved (or a new version approved),
push it to **each** PA agent that should know it:

```
PUT /agents/{agent_id}/knowledge/external/{key}          (API key scope: knowledge)
{"version": 3, "title": "Valuations are anchors",
 "content": "An agent's valuation is not the seller's minimum price; treat it as their anchor. …",
 "scope": "company", "source": "opsapi"}
```

- `key` is OpsAPI's knowledge id (`[A-Za-z0-9][A-Za-z0-9._-]{0,99}`, e.g. `kb-17`). It is stored as an
  ordinary knowledge file `ext/<key>@v<version>.md` whose text starts
  `[knowledge id=kb-17 version=3 scope=company source=opsapi]`, so retrieval (run-start injection and
  `knowledge_search`) finds it and the agent can cite `{"type":"knowledge","id":"kb-17"}`.
- **201** `created` (first version) or `replaced` (older versions retired, `removed_versions` = n);
  **200** `updated` (same version, content refreshed); **409** `{current_version}` when a newer version is
  already stored, so a late delivery cannot roll a rule back; **404** agent not in the key's org.
- Retiring a rule: `DELETE /agents/{agent_id}/knowledge/external/{key}` removes every version (204,
  idempotent).
- Re-embedding is best-effort, as for every knowledge write. Draft or unapproved rules must not be pushed.

Personal and team scopes: JobShout knowledge is per agent, not per user. Push only company rules to the
shared agents for now; personal rules should reach a run through `inputs` (see open questions).

## 7. Approvals

`email_send`, `slack_send_message` and `teams_send_message` are approval-gated on both agents. A call
pauses the run (`awaiting_approval`), emits `approval.requested` (with `tool_input`, so OpsAPI can show
the draft), and resumes after `POST /approvals/{id}/decide`. `opsapi_post` is **not** gated: its paths are
internal records that a human confirms inside OpsAPI. An admin can gate it per agent with an approval
rule; re-install keeps the rule.

## 8. Open questions

- **Credential storage**: integration credentials (the OpsAPI key) are plain JSONB and returned by
  `GET /integrations` to any org member. Seal them with `SECRETS_ENCRYPTION_KEY` and mask them in
  responses before production.
- **Routed model per run**: `model_provider`/`model_name` are the requested model; the hop that actually
  served the call under an org chain is not recorded on the run.
- **Per-user knowledge** (personal rules) needs either per-run knowledge or a user-scoped filter.
- **Write paths** match OpsAPI's property_pa plugin (`captures`, `handoffs`, `knowledge`, `grounding/check`); the agent's OpsAPI user has the `pd_agent` role, which can propose but never confirm.
- **Idempotency** of `opsapi_post`: no idempotency key is sent. If a write may be repeated (a retried
  turn), OpsAPI should dedupe, or we add an `Idempotency-Key` derived from run and tool call.
