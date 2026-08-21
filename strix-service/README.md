# JobShout Pentest Service

An HTTP front end for [Strix](https://github.com/usestrix/strix), so the JobShout
platform can run autonomous penetration tests against targets it is authorised to
test.

## Why this runs outside the cluster

Strix sandboxes every scan in Docker containers. A k3s pod has no Docker daemon,
and giving it one — privileged docker-in-docker — would mean the most privileged
container in the cluster is the one running attack tooling. That is the wrong
trade to make anywhere, and a particularly poor one here.

Three other things point the same way:

- **Scans run for minutes to hours.** The previous design launched one in a
  background goroutine inside the API pod, so any deploy, OOM or node drain lost
  the scan silently and left its database row stuck at `running` forever.
- **Results were written to one pod's disk.** With more than one replica, the pod
  that served the results request was rarely the pod that had them.
- **Reasoning cost money and left the network.** `STRIX_LLM=openai/gpt-4o` meant
  every scan was metered, and the prompts describing our own infrastructure's
  weak points went to a third party. On this machine the model is Ollama on
  `localhost`, so scans are free and the prompts stay here. That matters more for
  pentest than for anything else the platform runs.

This is the same arrangement the platform already uses for language models and
images: Ollama and the image service also run on the workstation, and every ring
reaches one instance of each over a JWT-gated public endpoint.

| | image-service | strix-service |
| --- | --- | --- |
| Why not the cluster | needs a Metal GPU | needs a real Docker daemon |
| Port | 11435 | 11436 |
| Shape | blocks ~25s, returns the image | start-and-poll |

## Endpoints

| Method | Path | Auth | Purpose |
| --- | --- | --- | --- |
| `GET` | `/health` | no | Liveness. Never probes anything. |
| `GET` | `/` | no | Service name, model, whether auth and scope are configured. |
| `GET` | `/api/capabilities` | yes | Strix, Docker and Ollama reachability; what is in scope. |
| `POST` | `/api/scan` | yes | Start a scan. Returns immediately. |
| `GET` | `/api/scan/{id}` | yes | Status and findings. Poll until terminal. |
| `GET` | `/api/scans` | yes | Recent runs, summaries only. |
| `DELETE` | `/api/scan/{id}` | yes | Cancel a run. |

`/health` is deliberately unauthenticated and deliberately does not touch Docker,
Strix or Ollama: a health check that can fail for four different reasons cannot
be used to diagnose any of them. Those checks live on `/api/capabilities`, which
is asked on purpose.

### Start and poll

A cover image takes about 25 seconds, so the image service blocks and returns the
picture. A scan takes minutes to hours — far too long to hold an HTTP request
open through Cloudflare and the pop0 edge. So:

```bash
POST /api/scan
{ "target": "https://juice.internal", "scan_mode": "quick",
  "max_budget": 10, "run_ref": "<pentest_runs.id>" }

→ 202 { "run_id": "…", "status": "queued", "queue_position": 0, "existing": false }
```

```bash
GET /api/scan/{run_id}
→ { "status": "completed", "finding_count": 3, "findings": [ … ],
    "exit_code": 2, "duration_ms": 412330, "log_tail": "…" }
```

Terminal statuses are `completed`, `failed`, `budget_exceeded` and `cancelled`.
They match the `pentest_runs.status` vocabulary in migration 026 exactly, so the
caller stores what it receives without translating it.

**`run_ref` is an idempotency key.** Send the caller's own run id and a repeat
POST returns the existing run with `200` and `"existing": true` rather than
starting a second scan. Without it, one dropped response during a retry costs a
duplicate hours-long scan against a live target.

## Scope — read this before deploying

This service takes *a target to attack*. An unguarded instance is an open
scanning relay for anyone who can reach it.

`STRIX_TARGET_ALLOWLIST` is the control, and it is enforced here rather than only
in the calling API: the caller's check can be bypassed by anyone who obtains a
token, while this one is the code path that decides whether a subprocess runs.

**Empty means deny everything, and empty is the default.** The service starts
and answers `/health`, but every scan request returns 403. An unconfigured
scanner that scans nothing is a non-event; one that scans anything reachable is
an incident with someone else's name on it.

Entries may be hosts, wildcards, CIDRs or URL prefixes:

```bash
STRIX_TARGET_ALLOWLIST='juice.internal,*.staging.example.com,10.13.0.0/24,https://app.example.com/beta'
```

A literal `*` allows any **public** host (private / metadata still refused). Use
it only for short bring-up windows. Int Product Phase 2 restores named hosts,
e.g. `int.jobshout.co.uk`.

Two independent gates, both of which must pass:

1. **The target matches a rule.** `*.example.com` covers `api.example.com` and
   `example.com`, but never `notexample.com`. A URL rule grants its path prefix
   and no sibling paths.
2. **Every address it resolves to is public, or named by a CIDR rule.** Gate 1
   alone trusts DNS: `staging.example.com` can be a perfectly legitimate entry
   and still resolve to `10.0.0.5` or `169.254.169.254`. A hostname rule cannot
   authorise an internal address — only a network rule can, because whoever
   wrote the hostname did not necessarily know where it pointed.

A name resolving to one public and one internal address is refused on the
internal one. `STRIX_ALLOW_PRIVATE_TARGETS=true` disables gate 2 entirely; do not
set it on a machine reachable from anywhere else.

Every accepted and refused target is logged with the calling app and `run_ref`.

## Authentication

An HS256 JWT in the **`x-api-key`** header — no `Bearer` prefix — carrying an
`app` claim, exactly what `server/internal/gatewayauth` mints for Ollama and the
image service.

Set it with `STRIX_JWT_SECRET`, and **use a different secret than the other two**:
they front a language model and a GPU, this one fronts a vulnerability scanner,
and a leak of one credential should not hand over all three. Use at least 32
bytes — PyJWT warns below that for HS256.

When the secret is unset the service accepts every request, which is what makes a
local development run work without ceremony. Note what does *not* follow: scope
is a separate gate, so an unauthenticated service still refuses every target
outside its allowlist. Unset auth widens who may ask, never what may be scanned.

## Running it

```bash
cd strix-service
STRIX_TARGET_ALLOWLIST=juice.internal ./run.sh          # foreground, port 11436
STRIX_JWT_SECRET=… STRIX_TARGET_ALLOWLIST=… ./run.sh    # authenticated
```

`run.sh` creates its own `.venv` with `uv` — unlike the image service there is no
large environment to borrow, because Strix is a subprocess rather than an import.

As a launchd agent, so it survives a logout and returns after a reboot:

```bash
STRIX_JWT_SECRET=… STRIX_TARGET_ALLOWLIST=… ./install-launchd.sh
```

The installer sets `PATH` explicitly in the plist. A launchd agent inherits a
minimal one, which is the usual reason a service that works perfectly in a shell
cannot find `strix` or `docker` once it is an agent.

## Configuration

| Variable | Default | Meaning |
| --- | --- | --- |
| `STRIX_TARGET_ALLOWLIST` | *(empty — denies everything)* | Hosts, wildcards, CIDRs or URL prefixes that may be scanned. |
| `STRIX_ALLOW_PRIVATE_TARGETS` | `false` | Permit internal addresses without naming their range. |
| `STRIX_JWT_SECRET` | *(empty — accepts everything)* | Gateway JWT secret. |
| `STRIX_BIN` | `strix` | Path to the Strix binary. |
| `STRIX_RUNS_DIR` | `./strix_runs` | Where artifacts are kept. |
| `STRIX_EXTRA_ARGS` | *(empty)* | Extra flags appended to every invocation. |
| `STRIX_LLM` | `ollama_chat/qwen3-coder:30b` | LiteLLM model id. |
| `STRIX_LLM_API_BASE` | `http://localhost:11434` | Local model endpoint. |
| `STRIX_LLM_API_KEY` | *(empty)* | Only for a hosted provider. |
| `STRIX_MAX_CONCURRENT` | `1` | Simultaneous scans. |
| `STRIX_QUEUE_MAX` | `8` | Waiting scans before 503. |
| `STRIX_MAX_RUNTIME_SECONDS` | `7200` | Wall-clock ceiling on one scan. |
| `STRIX_RETENTION_DAYS` | `14` | How long artifacts are kept. |
| `STRIX_PORT` | `11436` | Listen port. |

The model id needs the `ollama_chat/` prefix, not plain `ollama/` — that is the
one LiteLLM supports tool calling on, and without tool calling Strix cannot drive
anything. `qwen3-coder:30b` is already on this workstation.

Concurrency defaults to **1** because a scan is Docker containers plus a 30B
model on the same GPU that serves the rest of the platform; a second simultaneous
scan is contention rather than throughput. Beyond `STRIX_QUEUE_MAX` the service
answers `503` with a `Retry-After` — the request was valid and trying later is
the right response to it, so it is not a 500.

With a local model the money budget is effectively zero, which makes **time** the
budget that actually bounds a run. `STRIX_MAX_RUNTIME_SECONDS` terminates the
process group at the ceiling. Containers Strix left behind may need clearing with
`docker ps`; the run's error message says so.

## Retention

Artifacts older than `STRIX_RETENTION_DAYS` are deleted on startup. Findings are
already copied into Postgres by the caller, and Strix's output can contain
credentials it discovered — so keeping it indefinitely is a liability rather than
an archive. `strix_runs/` is gitignored for the same reason.

## Tests

```bash
uv pip install --python .venv/bin/python -r requirements-dev.txt
.venv/bin/python -m pytest
```

82 tests, no Strix or Docker required: the runner is exercised against a fake
binary that reproduces the only two things it depends on — an exit code, and a
`vulnerabilities.json` in the working directory.
