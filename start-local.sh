#!/usr/bin/env bash
#
# Run JobShout against a native Postgres — no Docker required.
#
# start-dev.sh assumes Docker: it starts compose services (including an
# `ollama` service that no longer exists) and expects Postgres on 5432 and
# MinIO on 9000, while docker-compose.yml maps 5433 and 9100. This script is
# the Docker-free path.
#
# Secrets come from .env, which is gitignored. The Go binary reads process
# environment only (viper.AutomaticEnv), so .env is exported here rather than
# read by the server itself.
#
#   ./start-local.sh          # backend + UI, tail logs, Ctrl+C to stop
#
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="$ROOT_DIR/.dev-logs"
mkdir -p "$LOG_DIR"

# Ports: 8080/8090/8095 are taken by Ring Promoter on this machine.
API_PORT="${API_PORT:-8181}"
UI_PORT="${UI_PORT:-3001}"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
else
  echo "[start-local] WARNING: no .env — Ollama gateway auth will be unset."
fi

PIDS=()
cleanup() {
  echo ""
  echo "[start-local] Stopping..."
  for pid in "${PIDS[@]}"; do
    kill "$pid" 2>/dev/null || true
  done
  wait 2>/dev/null || true
}
trap cleanup EXIT INT TERM

# ── Postgres ────────────────────────────────────────────────────────────────
if ! pg_isready -q 2>/dev/null; then
  echo "[start-local] Postgres is not running. Start it with:"
  echo "    brew services start postgresql@16"
  exit 1
fi

if ! psql -lqt 2>/dev/null | cut -d\| -f1 | grep -qw jobshout; then
  echo "[start-local] Creating the jobshout role and database..."
  psql -d postgres -c "CREATE ROLE jobshout LOGIN PASSWORD 'jobshout' SUPERUSER;" || true
  psql -d postgres -c "CREATE DATABASE jobshout OWNER jobshout;"
fi

# Migration 000016 opens with CREATE EXTENSION vector, and a failed migration
# is fatal at boot — so check for pgvector before starting rather than after.
if ! psql -d postgres -tAc \
  "SELECT 1 FROM pg_available_extensions WHERE name='vector';" | grep -q 1; then
  echo "[start-local] pgvector is missing for postgresql@16."
  echo "  The Homebrew bottle only ships pg17/pg18 binaries, so build it:"
  echo "    git clone --branch v0.8.0 --depth 1 https://github.com/pgvector/pgvector.git"
  echo "    cd pgvector && make && make install \\"
  echo "      PG_CONFIG=/opt/homebrew/opt/postgresql@16/bin/pg_config"
  exit 1
fi

# ── Backend ─────────────────────────────────────────────────────────────────
echo "[start-local] Building the API..."
(cd "$ROOT_DIR/server" && go build -o bin/jobshout-server ./cmd/server)

echo "[start-local] API on :$API_PORT  (logs: $LOG_DIR/server.log)"
(
  cd "$ROOT_DIR/server"
  DATABASE_URL="${DATABASE_URL:-postgres://jobshout:jobshout@localhost:5432/jobshout?sslmode=disable}" \
  JWT_SECRET="${JWT_SECRET:-dev-only-change-me-to-a-random-32-character-string}" \
  SERVER_PORT="0.0.0.0:$API_PORT" \
  CORS_ORIGINS="${CORS_ORIGINS:-http://localhost:$UI_PORT}" \
  MINIO_ENDPOINT="${MINIO_ENDPOINT:-}" \
  PYTHON_SIDECAR_URL="${PYTHON_SIDECAR_URL:-}" \
  ./bin/jobshout-server
) >"$LOG_DIR/server.log" 2>&1 &
PIDS+=($!)

# ── Frontend ────────────────────────────────────────────────────────────────
echo "[start-local] UI on :$UI_PORT  (logs: $LOG_DIR/ui.log)"
(
  cd "$ROOT_DIR/web/nextjs"
  [[ -d node_modules ]] || npm install
  NEXT_PUBLIC_API_URL="http://localhost:$API_PORT" \
  NEXT_PUBLIC_WS_URL="ws://localhost:$API_PORT" \
  npm run dev
) >"$LOG_DIR/ui.log" 2>&1 &
PIDS+=($!)

echo ""
echo "  UI:  http://localhost:$UI_PORT"
echo "  API: http://localhost:$API_PORT"
echo ""
echo "[start-local] Tailing logs. Ctrl+C to stop."
tail -n +1 -F "$LOG_DIR/server.log" "$LOG_DIR/ui.log"
