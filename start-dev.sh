#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
LOG_DIR="$ROOT_DIR/.dev-logs"
mkdir -p "$LOG_DIR"

if [[ -f "$ROOT_DIR/.env" ]]; then
  set -a
  # shellcheck disable=SC1091
  source "$ROOT_DIR/.env"
  set +a
fi

# Detect LAN IP so access from other machines on the VLAN works.
# Override by exporting HOST_IP before running.
if [[ -z "${HOST_IP:-}" ]]; then
  HOST_IP="$(ipconfig getifaddr en0 2>/dev/null || true)"
  if [[ -z "$HOST_IP" ]]; then
    HOST_IP="$(ipconfig getifaddr en1 2>/dev/null || true)"
  fi
  if [[ -z "$HOST_IP" ]]; then
    HOST_IP="localhost"
  fi
fi

# CORS must allow both localhost and the LAN origin for browsers hitting either.
DEFAULT_CORS="http://localhost:3001,http://${HOST_IP}:3001"

PIDS=()

cleanup() {
  echo ""
  echo "[start-dev] Stopping services..."
  for pid in "${PIDS[@]}"; do
    if kill -0 "$pid" 2>/dev/null; then
      kill "$pid" 2>/dev/null || true
    fi
  done
  wait 2>/dev/null || true
  echo "[start-dev] Done."
}
trap cleanup EXIT INT TERM

echo "[start-dev] Starting infra (postgres, minio, ollama, python-sidecar)..."
docker compose -f "$ROOT_DIR/docker-compose.yml" up -d postgres minio ollama python-sidecar

echo "[start-dev] Waiting for postgres..."
until docker compose -f "$ROOT_DIR/docker-compose.yml" exec -T postgres pg_isready -U "${POSTGRES_USER:-jobshout}" >/dev/null 2>&1; do
  sleep 1
done

SERVER_BIN="$ROOT_DIR/server/bin/jobshout-server"
echo "[start-dev] Building Go backend -> $SERVER_BIN"
(
  cd "$ROOT_DIR/server"
  mkdir -p bin
  go build -o "$SERVER_BIN" ./cmd/server
) >>"$LOG_DIR/server.log" 2>&1

echo "[start-dev] Starting Go backend on :8080 (logs: $LOG_DIR/server.log)"
(
  cd "$ROOT_DIR/server"
  DATABASE_URL="${DATABASE_URL:-postgres://jobshout:jobshout@localhost:5432/jobshout?sslmode=disable}" \
  JWT_SECRET="${JWT_SECRET:-dev-only-change-me-to-a-random-32-character-string}" \
  SERVER_PORT="${SERVER_PORT:-0.0.0.0:8080}" \
  CORS_ORIGINS="${CORS_ORIGINS:-$DEFAULT_CORS}" \
  FRONTEND_BASE_URL="${FRONTEND_BASE_URL:-http://${HOST_IP}:3001}" \
  MINIO_ENDPOINT="${MINIO_ENDPOINT:-localhost:9000}" \
  MINIO_ACCESS_KEY="${MINIO_ACCESS_KEY:-minioadmin}" \
  MINIO_SECRET_KEY="${MINIO_SECRET_KEY:-minioadmin}" \
  PYTHON_SIDECAR_URL="${PYTHON_SIDECAR_URL:-http://localhost:8001}" \
  OLLAMA_BASE_URL="${OLLAMA_BASE_URL:-http://localhost:11434}" \
  "$SERVER_BIN"
) >>"$LOG_DIR/server.log" 2>&1 &
PIDS+=($!)

echo "[start-dev] Starting Next.js UI on :3001 (logs: $LOG_DIR/ui.log)"
(
  cd "$ROOT_DIR/web/nextjs"
  if [[ ! -d node_modules ]]; then
    echo "[start-dev] Installing UI dependencies..."
    npm install
  fi
  NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-http://${HOST_IP}:8080}" \
  NEXT_PUBLIC_WS_URL="${NEXT_PUBLIC_WS_URL:-ws://${HOST_IP}:8080}" \
  HOSTNAME=0.0.0.0 \
  npm run dev -- -H 0.0.0.0
) >"$LOG_DIR/ui.log" 2>&1 &
PIDS+=($!)

echo ""
echo "[start-dev] Services running (bind 0.0.0.0; HOST_IP=${HOST_IP}):"
echo "  UI:         http://${HOST_IP}:3001   (also http://localhost:3001)"
echo "  API:        http://${HOST_IP}:8080   (also http://localhost:8080)"
echo "  PostgreSQL: ${HOST_IP}:5432"
echo "  MinIO:      http://${HOST_IP}:9000 (console :9001)"
echo "  Ollama:     http://${HOST_IP}:11434"
echo ""
echo "[start-dev] Tailing logs. Ctrl+C to stop."
tail -n +1 -F "$LOG_DIR/server.log" "$LOG_DIR/ui.log"
