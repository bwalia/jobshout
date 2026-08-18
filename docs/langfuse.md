# Langfuse LLM observability

Every LLM call the python-sidecar makes — LangChain runs, LangGraph workflows,
and both SSE streaming variants — can be traced to a self-hosted
[Langfuse](https://langfuse.com): full prompts and completions, per-model token
counts, latency, estimated cost, and errors, grouped per execution and per
agent.

## Quick start (local)

```bash
# 1. Start the Langfuse stack (opt-in compose profile; UI on :3002)
docker compose --profile langfuse up -d

# 2. Turn tracing on in the sidecar — add to .env, then restart python-sidecar
LANGFUSE_PUBLIC_KEY=pk-lf-jobshout-local
LANGFUSE_SECRET_KEY=sk-lf-jobshout-local

# 3. Provision the dashboard and local-model cost estimates
./scripts/langfuse_dashboard.py
./scripts/langfuse_models.py
```

Open http://localhost:3002 and sign in with `admin@jobshout.local` /
`jobshout-langfuse` (dev defaults; both overridable in `.env`). The dashboard
lives under **Dashboards → JobShout LLM Observability**.

First boot self-provisions the org, project, admin user and API keys via
`LANGFUSE_INIT_*` (see `docker-compose.yml`), so there is no signup step. All
secrets in the profile are development defaults — override every `LANGFUSE_*`
value in `.env` before pointing anything shared at it.

## What gets traced, and how it's labelled

`python-sidecar/app/observability.py` is the whole integration. Tracing is off
unless both keys are set; without them every code path behaves exactly as
before. When on:

| Langfuse field | JobShout value | Why |
|---|---|---|
| Trace name | `langchain-run`, `langgraph-run`, `langchain-stream`, `langgraph-stream` | which engine + endpoint ran |
| Session | `execution_id` | retries of one execution group together |
| User | `agent_id` | the dashboard slices call volume by agent |
| Tags | provider, model | quick filtering |

Attributes are applied with `propagate_attributes(...)` around the run rather
than invoke metadata, because only the context manager reaches *child* spans —
a LangGraph run nests its generations under per-node chains, and metadata-only
attribution leaves them showing as `n/a` in the by-agent widget.

## The dashboard

`scripts/langfuse_dashboard.py` creates it through the public API (dashboard
CRUD lives on the `/api/public/unstable` surface), so it can be re-provisioned
onto any Langfuse deployment. It is idempotent: widgets are matched by name
and reused, an existing dashboard is left alone unless `--force` is passed.

Widgets: LLM calls / total tokens / total cost stat tiles, calls over time by
model, p95 latency by model, calls by agent, calls by engine, tokens over
time, and errors over time. All count `GENERATION` observations only, so
LangGraph's one-span-per-node bookkeeping doesn't inflate the numbers.

## Cost for local models

Langfuse only prices generations whose model matches a price definition, and
workstation Ollama models are not in its built-in list — so cost reads $0.00
until `./scripts/langfuse_models.py` registers estimates. The defaults are
electricity-only Apple-silicon estimates (~$0.17 per 1M output tokens for a
30B q4 model; assumptions documented in the script) — tune the `PRICE_*`
constants to your machine and tariff. Two caveats:

- Prices apply to generations ingested **after** registration, not
  retroactively.
- Local inference is cheap: the "Total cost (USD)" tile rounds to cents and
  reads $0.00 until real volume accumulates. Per-trace costs show full
  precision in the Tracing view.

## Ports and services

The profile adds `langfuse-web` (host port `3002`, in-network `3000`),
`langfuse-worker`, `langfuse-clickhouse`, `langfuse-redis`, and
`langfuse-postgres` (Langfuse needs plain Postgres; the app one is pgvector
with a single database). It reuses the app MinIO with a dedicated `langfuse`
bucket, created by the one-shot `langfuse-minio-init` container.

A sidecar running *outside* compose (e.g. `uvicorn` during development) needs
`LANGFUSE_HOST=http://localhost:3002`; the compose default is the in-network
`http://langfuse-web:3000`.

## Deployment note

The k8s/helm chart intentionally doesn't deploy Langfuse yet. The sidecar
only needs the three `LANGFUSE_*` env vars pointed at any Langfuse deployment
(self-hosted or cloud), and both scripts accept `--host`/`--public-key`/
`--secret-key` to provision the dashboard and model prices there.
