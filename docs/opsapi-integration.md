# OpsAPI integration (Property PA)

OpsAPI's Property PA uses JobShout to run its agents and route its model calls. OpsAPI stays the system of record. It calls JobShout with an org API key, and JobShout calls back through signed webhooks. Each OpsAPI tenant maps to one JobShout **org**. Every resource below is scoped to that org; a resource that belongs to another org returns `404`.

All paths are relative to `/api/v1`.

## 1. API keys (service access)

Org admins (users with the `org:manage` permission) manage keys:

| Method | Path | Notes |
|---|---|---|
| `GET` | `/api-keys` | Lists keys: prefix, scopes, `last_used_at`, `expires_at`, `revoked_at`. |
| `POST` | `/api-keys` | Body: `{"name": "opsapi-prod", "scopes": ["runs","approvals","agents:read","knowledge"], "expires_at": null}`. The response includes `key` **once**. |
| `DELETE` | `/api-keys/{keyID}` | Revokes the key immediately. |

- **Key format:** `jsk_<8 hex>_<43 chars>`. The `jsk_<8 hex>` part is the display prefix. JobShout stores only the key's SHA-256, so a lost key can't be recovered. Revoke it and create a new one.
- **Using a key:** send `Authorization: Bearer jsk_...`. A key acts in its org **as the user who created it**: runs record that user as `requested_by`, and decisions record that user as `decided_by`. The request gets the role `service`. If the creator is deleted or leaves the org, the key stops working.
- **Scopes and routes:** a key can call only the routes below, and only when it has the matching scope. Every other route returns `403` to a key, including key and webhook management, routing, and agent create/update/delete. JWT auth for users is unchanged.

| Scope | Routes |
|---|---|
| `runs` | `POST /tasks/launch`, `POST /tasks/{id}/run`, `GET /tasks/{id}/runs`, `GET /task-runs/{id}` |
| `approvals` | `GET /approvals`, `GET /approvals/{id}`, `POST /approvals/{id}/decide` |
| `agents:read` | `GET /agents`, `GET /agents/{id}`, `GET /agent-schemas`, `GET /agent-schemas/generic` |
| `knowledge` | `GET` and `POST /agents/{id}/knowledge`; `GET`, `PUT` and `DELETE /agents/{id}/knowledge/{fileID}` |

The allow-list lives in `server/internal/middleware/apikey_auth.go` (`apiKeyRoutes`).

## 2. Model routing (per org)

By default an org uses the server's env providers (`LLM_PROVIDER`, `OPENAI_*`, `OLLAMA_*`, ...). Once the org has an **active** provider config (`/llm-providers`), every model call made for that org goes through the org's own chain instead:

```
POST /llm-providers   {"name":"OpenRouter","provider_type":"openai",
                       "base_url":"https://openrouter.ai/api/v1",
                       "api_key":"...","default_model":"anthropic/claude-sonnet-4.5","is_default":true}
POST /llm-providers/{id}/test   {"model": "optional override"}   → {ok, latency_ms, model, reply|error}
GET  /llm-routing                → {source: "org"|"env", enabled, chain, effective}
PUT  /llm-routing  (org:manage)  {"enabled": true,
                                   "chain": [{"provider_config_id":"<A>","model":"gpt-4.1"},
                                             {"provider_config_id":"<B>"}]}
```

- **Provider types:**
  - `openai`: any OpenAI-compatible endpoint (OpenAI, OpenRouter, vLLM, Groq, LM Studio). `base_url` works with or without `/v1`.
  - `claude`, `gemini`: use the API key.
  - `ollama`: uses `base_url` and no key.
- **Chain:** with no `/llm-routing` row, the chain is every active config, with the default config first. An explicit `chain` sets the order. Each hop can name a `model`; without one, the hop uses its config's `default_model`. `enabled: false` sends the org back to the env providers but keeps its configs.
- **Fallback:** if a hop fails for any reason (rate limit, outage, auth error, unknown model), the call moves to the next hop. If every hop fails, the call fails with `llm.ErrOrgChainExhausted`. The env `LLM_OLLAMA_FALLBACK` is **not** applied to org chains: the org's chain is its whole fallback policy.
- **Swapping models:** the hop's model replaces the agent's own model, so you change models by changing config only. Chains are cached for 30 seconds, and any write to a provider config or to the routing settings clears the cache.
- **Coverage:** executor runs (including resumes after an approval) and the specialist agents' routed clients. Not covered: embeddings, and the chat assistant's own client.
- **Secrets:** API keys are sealed with AES-256-GCM when `SECRETS_ENCRYPTION_KEY` is set. Older plaintext rows still read and are sealed the next time they are written.

## 3. Webhooks

Org admins manage endpoints:

| Method | Path | Notes |
|---|---|---|
| `POST` | `/webhook-endpoints` | Body: `{"url": "...", "events": [...], "secret": "optional, ≥16 chars", "description": "..."}`. The response includes `secret` **once**; JobShout generates one if you omit it. |
| `GET` | `/webhook-endpoints` | Lists endpoints. Secrets are never returned. |
| `GET` | `/webhook-endpoints/{id}` | One endpoint. |
| `PUT` | `/webhook-endpoints/{id}` | Changes `url`, `events`, `description` or `enabled`. `"rotate_secret": true` returns the new secret. |
| `DELETE` | `/webhook-endpoints/{id}` | Deletes the endpoint. |
| `GET` | `/webhook-endpoints/{id}/deliveries?limit=50` | The delivery log. |
| `POST` | `/webhook-endpoints/{id}/test` | Sends a signed `ping` and returns the delivery. |

**Events:**

| Event | When |
|---|---|
| `run.completed` | A task run's status changes to `completed`. |
| `run.failed` | A task run's status changes to `failed`. |
| `approval.requested` | A gated tool call is waiting for a human decision. |
| `approval.decided` | A decision is recorded. |

Use `"*"` to subscribe to all of them. Runs that resume after an approval send their `run.*` event when they finish.

Each delivery is a `POST` with this body:

```json
{"id":"<event uuid>","type":"run.completed","created_at":"…","org_id":"…","data":{…}}
```

- **Run `data` fields:**
  - IDs: `run_id`, `task_id`, `agent_id`, `execution_id`.
  - Result: `status`, `output` (truncated at 64 KiB, with `output_truncated: true`), `error_message`.
  - Request: `context`, `skill_slugs`, `inputs`.
  - Usage: `total_tokens`, `cost_usd`, `latency_ms`.
  - Timing: `started_at`, `completed_at`.
- **Approval `data` fields:**
  - IDs: `approval_id`, `execution_id`, `agent_id`, `task_run_id` (when it can be resolved).
  - The tool call: `tool_name`, `tool_input`.
  - The decision: `status`, `reason`, `requested_at`, `decided_by`, `decided_at`.

Headers: `X-JobShout-Event`, `X-JobShout-Event-Id`, `X-JobShout-Delivery`, `X-JobShout-Timestamp` (unix seconds), and `X-JobShout-Signature: sha256=<hex>`.

**Verifying a delivery:** compute `HMAC-SHA256(secret, "<X-JobShout-Timestamp>.<raw body>")`, hex-encode it, and compare it to the header in constant time. Reject timestamps more than about 5 minutes old.

**Retries:** any non-2xx response or network error is retried after 30s, 2m, 10m, 30m and 2h, for 6 attempts in total. After that the delivery is marked `failed`. Redirects are not followed. Retries are leased in the database, so several replicas are safe. Delivery is at-least-once, so **deduplicate on the envelope `id`**. Endpoint secrets are sealed at rest when `SECRETS_ENCRYPTION_KEY` is set.

## 4. Role context

One agent can serve several roles. Pass a `context` when starting a run:

```
POST /tasks/{taskID}/run
{"prompt":"Draft a reply to the buyer",
 "context":{"role":"seller_agent","user_ref":"opsapi-user-7","workspace_ref":"property-42"},
 "skill_slugs":["seller-voice"],
 "inputs":{"listing_id":"L-1"}}

POST /tasks/launch
{"agent_id":"…","project_id":"…","values":{"prompt":"…"},
 "context":{"role":"manager","user_ref":"…","workspace_ref":"…"},"skill_slugs":["portfolio-review"]}
```

- **How it reaches the agent:** the context is merged into the run's `inputs` as `role`, `user_ref` and `workspace_ref`. Those keys override inputs with the same name. The prompt's `## Inputs` block therefore tells the agent which role it is acting in.
- **Where it is stored:** `task_runs.run_context`. It comes back as `context` on `GET /task-runs/{id}` and in run webhooks.
- **Skills:** `skill_slugs` loads extra skills for that run only. Use per-role skills to give each role its own instructions.
- **What JobShout does with the refs:** nothing. They are opaque references; JobShout never resolves them.
- **Launch:** on `/tasks/launch`, `context` and `skill_slugs` apply to generic agents (task runs). Specialist modules ignore them.

## Settings and migrations

| Env | Purpose |
|---|---|
| `SECRETS_ENCRYPTION_KEY` | Seals org provider API keys and webhook secrets (64 hex characters are used as a raw key; any other value is hashed). If unset, they are stored in plaintext and the server logs a warning at boot. Changing this key makes existing sealed secrets unreadable. |

| Migration | Adds |
|---|---|
| `000063_org_api_keys` | `org_api_keys` |
| `000064_org_webhooks` | `org_webhook_endpoints`, `org_webhook_deliveries` |
| `000065_org_llm_routing` | `org_llm_routing` |
| `000066_task_run_context` | `task_runs.run_context` |

Postgres tests for all of these run when `PLATFORM_TEST_DATABASE_URL=postgres://…/postgres` is set. They create a throwaway database and apply every migration twice.
