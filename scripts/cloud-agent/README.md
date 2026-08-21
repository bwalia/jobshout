# Cloud Agent development environment

Scripts that make JobShout usable in a [Cursor Cloud Agent](https://cursor.com/docs/cloud-agent/setup)
without Docker. They run the core stack natively: **PostgreSQL 16 + pgvector**,
the **Go API**, and the **Next.js** UI. MinIO and the Python sidecar are
optional and left disabled by default (the API runs fine without them).

## Base environment

The environment's base snapshot provides the system toolchains:

- **Go 1.25** at `/usr/local/go` (also on `PATH` via `/etc/profile.d/go.sh`)
- **PostgreSQL 16** + **pgvector 0.8** (from the PGDG apt repo)
- **Node 20+** (from the default image)

## Lifecycle scripts

| Phase | Command | What it does |
| --- | --- | --- |
| `install` | `bash scripts/cloud-agent/install.sh` | Idempotent: `go mod download`, build the server, `npm ci`. |
| `start` | `bash scripts/cloud-agent/start.sh` | Starts PostgreSQL, ensures the `jobshout` db + `vector` extension, and launches the API (`:8080`) and UI (`:3001`) in the background (logs in `.dev-logs/`). |

`run-api.sh` and `run-web.sh` run a single service in the foreground and are used
by `start.sh`; run them directly to iterate on one service with live logs.

## URLs

- UI: http://localhost:3001
- API: http://localhost:8080 (health: `/health`, REST under `/api/v1`)

The Go API reads configuration from the process environment (see
`server/internal/config/config.go`); only `DATABASE_URL` and `JWT_SECRET` are
required, and `run-api.sh` supplies sensible local defaults for both.
