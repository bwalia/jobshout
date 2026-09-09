#!/usr/bin/env bash
#
# Cloud Agent start phase — per-boot bring-up of the full local dev stack.
#
# 1. Starts the native PostgreSQL 16 cluster (pgvector) the Go API needs.
# 2. Ensures the jobshout role/database/extension exist.
# 3. Launches the Go API (:8080) and the Next.js dev server (:3001) in the
#    background, writing logs to .dev-logs/.
#
# Everything here is idempotent: re-running it starts only what is not already
# up, so it is safe on every boot and after a restart.
set -euo pipefail

export PATH="/usr/local/go/bin:$PATH"
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
LOG_DIR="$ROOT_DIR/.dev-logs"
mkdir -p "$LOG_DIR"

echo "[start] starting PostgreSQL 16 cluster"
sudo pg_ctlcluster 16 main start 2>/dev/null || true

echo "[start] waiting for PostgreSQL to accept connections"
for _ in $(seq 1 30); do
  if pg_isready -h localhost -p 5432 -q; then
    break
  fi
  sleep 1
done
pg_isready -h localhost -p 5432

echo "[start] ensuring jobshout role, database and pgvector extension"
sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='jobshout'" | grep -q 1 \
  || sudo -u postgres psql -c "CREATE ROLE jobshout LOGIN PASSWORD 'jobshout' SUPERUSER;"
sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='jobshout'" | grep -q 1 \
  || sudo -u postgres createdb -O jobshout jobshout
sudo -u postgres psql -d jobshout -c "CREATE EXTENSION IF NOT EXISTS vector;" >/dev/null

# port_in_use PORT -> 0 if something is already listening (skip relaunch).
port_in_use() {
  local port="$1"
  if command -v ss >/dev/null 2>&1; then
    ss -ltn "( sport = :$port )" 2>/dev/null | grep -q ":$port"
  else
    curl -s -o /dev/null "http://localhost:$port" 2>/dev/null
  fi
}

if port_in_use 8080; then
  echo "[start] Go API already running on :8080"
else
  echo "[start] launching Go API on :8080 (logs: .dev-logs/server.log)"
  nohup bash "$ROOT_DIR/scripts/cloud-agent/run-api.sh" >"$LOG_DIR/server.log" 2>&1 &
fi

if port_in_use 3001; then
  echo "[start] Next.js UI already running on :3001"
else
  echo "[start] launching Next.js UI on :3001 (logs: .dev-logs/ui.log)"
  nohup bash "$ROOT_DIR/scripts/cloud-agent/run-web.sh" >"$LOG_DIR/ui.log" 2>&1 &
fi

echo "[start] stack up — API http://localhost:8080  UI http://localhost:3001"
