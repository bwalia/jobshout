# Cloud Agent development environment

Scripts that make JobShout usable in a [Cursor Cloud Agent](https://cursor.com/docs/cloud-agent/setup)
without Docker. They run the core stack natively: **PostgreSQL 16 + pgvector**,
the **Go API**, and the **Next.js** UI. MinIO and the Python sidecar are
optional and left disabled by default (the API runs fine without them).

The scripts are self-contained: `install.sh` provisions the system toolchains
that are not in the default base image, so no custom base snapshot is required.

## Toolchains (provisioned by `install.sh`)

- **Go 1.25** at `/usr/local/go` (also on `PATH` via `/etc/profile.d/go.sh`)
- **PostgreSQL 16** + **pgvector** (from the PGDG apt repo)
- **Node** (from the default base image)

## Lifecycle scripts

| Phase | Command | What it does |
| --- | --- | --- |
| `install` | `bash scripts/cloud-agent/install.sh` | Idempotent: installs Go 1.25 and PostgreSQL 16 + pgvector if missing, then `go mod download`, builds the server, and runs `npm ci`. |
| `start` | `bash scripts/cloud-agent/start.sh` | Starts PostgreSQL, ensures the `jobshout` db + `vector` extension, and launches the API (`:8080`) and UI (`:3001`) in the background (logs in `.dev-logs/`). |

`run-api.sh` and `run-web.sh` run a single service in the foreground and are used
by `start.sh`; run them directly to iterate on one service with live logs.

## URLs

- UI: http://localhost:3001
- API: http://localhost:8080 (health: `/health`, REST under `/api/v1`)

The Go API reads configuration from the process environment (see
`server/internal/config/config.go`); only `DATABASE_URL` and `JWT_SECRET` are
required, and `run-api.sh` supplies sensible local defaults for both.
