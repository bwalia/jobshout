# Langfuse LLM observability

Every LLM call the python-sidecar makes — LangChain runs, LangGraph workflows,
and both SSE streaming variants — can be traced to a self-hosted
[Langfuse](https://langfuse.com): full prompts and completions, per-model token
counts, latency, estimated cost, and errors, grouped per execution and per
agent.

## Deployment on k3s (the main path)

The Helm chart deploys the whole Langfuse stack per ring behind
`langfuse.enabled` — langfuse web + worker, ClickHouse, and a dedicated
Postgres, Redis and MinIO (see the `langfuse:` block in
`deploy/helm/jobshout/values.yaml` for why each is separate from the app's).
It is **on in int, off elsewhere**; enabling another ring is a one-line change
in that ring's `values-<env>.yaml`.

Merging to master deploys it through the usual pipeline (CI → Ring Promoter
int → `helm upgrade`). On first deploy every credential — SALT, encryption
key, store passwords, the admin login, the project API keys — is generated
in-cluster and persisted in the `jobshout-langfuse-secrets` Secret, so nothing
secret lives in this public repo and there is no bootstrap step. The sidecar
reads the same Secret, so tracing is live ring-wide immediately.

The UI is served on its own host, `https://int-langfuse.jobshout.co.uk`
(login: `admin@jobshout.local`; password from the Secret):

```bash
kubectl -n int get secret jobshout-langfuse-secrets \
  -o jsonpath='{.data.LANGFUSE_INIT_USER_PASSWORD}' | base64 -d
```

Two one-time steps after the first deploy of a ring:

1. **Edge registration** (until then the host answers "Host not configured"):
   run the *Register edge vhost* workflow with
   `host=int-langfuse.jobshout.co.uk`,
   `server_spec=deploy/edge/wslproxy-server-langfuse-int.json`,
   `health_path=/auth/sign-in`.
2. **Provision the dashboard + model prices** against the public host, with
   the keys read from the same Secret:

   ```bash
   PK=$(kubectl -n int get secret jobshout-langfuse-secrets -o jsonpath='{.data.LANGFUSE_PUBLIC_KEY}' | base64 -d)
   SK=$(kubectl -n int get secret jobshout-langfuse-secrets -o jsonpath='{.data.LANGFUSE_SECRET_KEY}' | base64 -d)
   ./scripts/langfuse_dashboard.py --host https://int-langfuse.jobshout.co.uk --public-key "$PK" --secret-key "$SK"
   ./scripts/langfuse_models.py    --host https://int-langfuse.jobshout.co.uk --public-key "$PK" --secret-key "$SK"
   ```

   Dashboards live in Langfuse's own Postgres, so this survives every
   subsequent deploy — it is per ring, not per release.

The dashboard is then at **Dashboards → JobShout LLM Observability** on that
host.

Note on the very first deploy: it pulls ~2.5 GB of new images (ClickHouse,
langfuse web/worker) onto the nodes, which can overrun Ring Promoter's
`helm --wait` 5-minute budget and mark the release failed while the pods go on
to converge — the same cosmetic failure mode the API's startup-probe comment
in values.yaml describes. If that happens, re-seed the same version; the
second attempt deploys against cached images.

## Quick start (local, docker compose)

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

## Pointing a ring at an external Langfuse instead

The sidecar only needs `LANGFUSE_HOST`, `LANGFUSE_PUBLIC_KEY` and
`LANGFUSE_SECRET_KEY`. In a ring with `langfuse.enabled: false` the chart sets
none of them, so an operator-created `extraSecretRefs` secret carrying those
three keys points tracing at any external Langfuse (another ring's, or cloud)
without deploying the stack there. Both scripts accept
`--host/--public-key/--secret-key` to provision the dashboard and model prices
on whatever host is used.
