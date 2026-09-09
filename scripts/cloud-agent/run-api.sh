#!/usr/bin/env bash
#
# Cloud Agent terminal — the Go API server (chi, pgx, JWT) on :8080.
#
# MinIO and the Python sidecar are optional and left unset so the API runs
# self-contained against PostgreSQL only; set MINIO_ENDPOINT / PYTHON_SIDECAR_URL
# to enable object storage and LangChain/LangGraph execution.
set -euo pipefail

export PATH="/usr/local/go/bin:$PATH"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

export DATABASE_URL="${DATABASE_URL:-postgres://jobshout:jobshout@localhost:5432/jobshout?sslmode=disable}"
export JWT_SECRET="${JWT_SECRET:-dev-only-change-me-to-a-random-32-character-string}"
export SERVER_PORT="${SERVER_PORT:-0.0.0.0:8080}"
export CORS_ORIGINS="${CORS_ORIGINS:-http://localhost:3001}"
export MINIO_ENDPOINT="${MINIO_ENDPOINT:-}"
export PYTHON_SIDECAR_URL="${PYTHON_SIDECAR_URL:-}"

cd "$ROOT_DIR/server"
exec go run ./cmd/server
