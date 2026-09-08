#!/bin/bash
#
# JobShout local development environment.
#
# Brings up the compose infra (Postgres + pgvector, MinIO), builds and runs the
# Go API, runs the Next.js UI, and tails both logs.
#
# Do NOT run this with sudo. Under sudo the Go build, npm and Next all write
# root-owned files into the repo — web/nextjs/.next in particular — and every
# later run as yourself then dies on EACCES. The preflight repairs that damage
# if it has already happened, but the fix is to not use sudo in the first place.

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# ============================================
# Configuration
# ============================================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMPOSE_FILE="$SCRIPT_DIR/docker-compose.yml"
ENV_FILE="$SCRIPT_DIR/.env"
WEB_DIR="$SCRIPT_DIR/web/nextjs"
SERVER_DIR="$SCRIPT_DIR/server"
# JobShout.com is a separate product in the same repo: its own compose stack
# (Rust API, Postgres, Redis, NATS, MinIO) and its own Next.js front end.
COM_DIR="$SCRIPT_DIR/jobshout-com"
COM_WEB_DIR="$COM_DIR/web/nextjs"
COM_COMPOSE_FILE="$COM_DIR/docker-compose.yml"
LOG_DIR="$SCRIPT_DIR/.dev-logs"

# Ports. 8080/8090/8095 are taken by Ring Promoter on this machine.
API_PORT="${API_PORT:-8181}"
UI_PORT="${UI_PORT:-3001}"
PG_PORT="${POSTGRES_HOST_PORT:-5433}"
MINIO_PORT="${MINIO_API_HOST_PORT:-9100}"
MINIO_CONSOLE_PORT="${MINIO_CONSOLE_HOST_PORT:-9110}"

# JobShout.com. Every port here is distinct from the platform's above, so both
# stacks can run at once without either noticing the other.
COM_WEB_PORT="${COM_WEB_PORT:-3010}"
COM_API_PORT="${COM_API_PORT:-8088}"
COM_PG_PORT="${COM_PG_PORT:-5434}"
COM_MINIO_CONSOLE_PORT="${COM_MINIO_CONSOLE_PORT:-9011}"

API_PID_FILE="$LOG_DIR/api.pid"
UI_PID_FILE="$LOG_DIR/ui.pid"
COM_WEB_PID_FILE="$LOG_DIR/com-web.pid"

# ============================================
# Usage and Help
# ============================================
show_help() {
    echo -e "${GREEN}JobShout Local Development Environment${NC}"
    echo ""
    echo -e "${BLUE}Usage:${NC}"
    echo "  ./start-local.sh [OPTIONS]"
    echo ""
    echo -e "${BLUE}Options:${NC}"
    echo "  -d, --detach          Run in background (don't attach to logs)"
    echo "  -r, --reset           Reset the database (removes volumes, wipes data)"
    echo "      --no-infra        Skip docker compose (infra already running)"
    echo "      --no-com          Skip the JobShout.com stack (platform only)"
    echo "      --com-only        Run only the JobShout.com stack"
    echo "      --ui-port PORT    UI port (default $UI_PORT)"
    echo "      --api-port PORT   API port (default $API_PORT)"
    echo "      --stop            Stop API, UI and containers, then exit"
    echo "      --clean           Stop everything and remove volumes, then exit"
    echo "      --status          Show what is running, then exit"
    echo "      --logs            Tail the API and UI logs, then exit"
    echo "  -h, --help            Show this help message"
    echo ""
    echo -e "${BLUE}Examples:${NC}"
    echo "  ./start-local.sh                    # Start everything, tail logs"
    echo "  ./start-local.sh -d                 # Start in the background"
    echo "  ./start-local.sh -r                 # Fresh database, then start"
    echo "  ./start-local.sh --ui-port 3005     # Run the UI on another port"
    echo "  ./start-local.sh --no-com           # Platform only, skip JobShout.com"
    echo "  ./start-local.sh --com-only         # Only JobShout.com"
    echo "  ./start-local.sh --stop             # Stop everything"
    echo "  ./start-local.sh --status           # What is up right now?"
    echo ""
    echo -e "${BLUE}Access URLs:${NC}"
    echo -e "  ${CYAN}Platform${NC}"
    echo "    UI:            http://localhost:$UI_PORT"
    echo "    API:           http://localhost:$API_PORT"
    echo "    API health:    http://localhost:$API_PORT/health"
    echo "    MinIO console: http://localhost:$MINIO_CONSOLE_PORT  (minioadmin / minioadmin)"
    echo "    PostgreSQL:    localhost:$PG_PORT  (user: jobshout / jobshout)"
    echo ""
    echo -e "  ${CYAN}JobShout.com${NC}"
    echo "    Web:           http://localhost:$COM_WEB_PORT"
    echo "    API:           http://localhost:$COM_API_PORT"
    echo "    MinIO console: http://localhost:$COM_MINIO_CONSOLE_PORT"
    echo "    PostgreSQL:    localhost:$COM_PG_PORT"
    echo ""
    echo -e "${BLUE}Hot-Reload (no restart needed):${NC}"
    echo "  web/nextjs/**              Next.js Fast Refresh updates on save"
    echo "  public/landing/**          Static files, just refresh the browser"
    echo "  server/**  (Go)            Needs a restart — it is a compiled binary"
    echo "  .env                       Needs a restart — read once at boot"
    echo ""
}

# ============================================
# Parse Arguments
# ============================================
DETACH=false
RESET=false
NO_INFRA=false
NO_COM=false
COM_ONLY=false
ACTION=""

while [[ $# -gt 0 ]]; do
    case "$1" in
        -d|--detach)   DETACH=true; shift ;;
        -r|--reset)    RESET=true; shift ;;
        --no-infra)    NO_INFRA=true; shift ;;
        --no-com)      NO_COM=true; shift ;;
        --com-only)    COM_ONLY=true; shift ;;
        --ui-port)     UI_PORT="$2"; shift 2 ;;
        --api-port)    API_PORT="$2"; shift 2 ;;
        --stop)        ACTION="stop"; shift ;;
        --clean)       ACTION="clean"; shift ;;
        --status)      ACTION="status"; shift ;;
        --logs)        ACTION="logs"; shift ;;
        -h|--help)     show_help; exit 0 ;;
        *)
            echo -e "${RED}Unknown option: $1${NC}"
            show_help
            exit 1
            ;;
    esac
done

mkdir -p "$LOG_DIR"

# ============================================
# Shared helpers
# ============================================
say()  { echo -e "${GREEN}[+]${NC} $*"; }
warn() { echo -e "${YELLOW}[!]${NC} $*"; }
info() { echo -e "${BLUE}[i]${NC} $*"; }
fail() { echo -e "${RED}[!]${NC} $*"; }

port_busy() {
    lsof -nP -iTCP:"$1" -sTCP:LISTEN -t >/dev/null 2>&1 && return 0
    # lsof cannot see a socket owned by another user; netstat still lists it.
    netstat -anv -p tcp 2>/dev/null | awk -v p="\\.$1\$" '$4 ~ p && $6=="LISTEN"' | grep -q .
}

# Free a port. A leftover dev server is the most confusing failure here: the new
# one loses the bind, exits, and the browser keeps being served stale code by a
# process nobody remembers starting.
free_port() {
    local port="$1" label="$2" pids owner parents
    pids="$(lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null)"
    if [[ -n "$pids" ]]; then
        say "Freeing $label port $port (pid $(echo "$pids" | tr '\n' ' '))"
        kill -9 $pids 2>/dev/null
        sleep 1
    fi

    port_busy "$port" || return 0

    # Still held means another user owns it — in practice root, from a past
    # `sudo ./start-local.sh`. sudo lsof can name the exact pid.
    warn "$label port $port is held by another user's process (a past sudo run)."
    info "Asking for your password to stop it."
    owner="$(sudo lsof -nP -iTCP:"$port" -sTCP:LISTEN -t 2>/dev/null)"
    if [[ -z "$owner" ]]; then
        fail "Could not identify it. Stop it by hand, then re-run."
        exit 1
    fi
    # `next dev` supervises a `next-server` child; killing only the child lets
    # the parent put another one straight back on the port.
    parents="$(ps -o ppid= -p $owner 2>/dev/null | tr -d ' ' | grep -v '^1$')"
    sudo kill -9 $owner $parents 2>/dev/null
    sleep 2
    if port_busy "$port"; then
        fail "Port $port is still held. Stop it by hand, then re-run."
        exit 1
    fi
}

stop_services() {
    local pid
    for f in "$API_PID_FILE" "$UI_PID_FILE" "$COM_WEB_PID_FILE"; do
        [[ -f "$f" ]] || continue
        pid="$(cat "$f" 2>/dev/null)"
        [[ -n "$pid" ]] && kill "$pid" 2>/dev/null
        rm -f "$f"
    done
    # Next spawns children that outlive the npm wrapper, so clear by port too.
    for p in "$API_PORT" "$UI_PORT" "$COM_WEB_PORT"; do
        pid="$(lsof -nP -iTCP:"$p" -sTCP:LISTEN -t 2>/dev/null)"
        [[ -n "$pid" ]] && kill -9 $pid 2>/dev/null
    done
}

# ============================================
# Quick Actions (exit after)
# ============================================
if [[ "$ACTION" == "stop" ]]; then
    say "Stopping JobShout services..."
    stop_services
    docker compose -f "$COMPOSE_FILE" stop postgres minio >/dev/null 2>&1
    [[ -f "$COM_COMPOSE_FILE" ]] && docker compose -f "$COM_COMPOSE_FILE" stop >/dev/null 2>&1
    say "Stopped."
    exit 0
fi

if [[ "$ACTION" == "clean" ]]; then
    warn "Stopping services and removing volumes (this wipes the database)..."
    stop_services
    docker compose -f "$COMPOSE_FILE" down -v
    [[ -f "$COM_COMPOSE_FILE" ]] && docker compose -f "$COM_COMPOSE_FILE" down -v
    say "Stopped and volumes removed."
    exit 0
fi

if [[ "$ACTION" == "status" ]]; then
    echo ""
    docker compose -f "$COMPOSE_FILE" ps 2>/dev/null
    echo ""
    if curl -sf "http://localhost:$API_PORT/health" >/dev/null 2>&1; then
        say "API is up on http://localhost:$API_PORT"
    else
        warn "API is not responding on port $API_PORT"
    fi
    if curl -sf "http://localhost:$UI_PORT/" >/dev/null 2>&1; then
        say "UI is up on http://localhost:$UI_PORT"
    else
        warn "UI is not responding on port $UI_PORT"
    fi
    if curl -sf "http://localhost:$COM_WEB_PORT/" >/dev/null 2>&1; then
        say "JobShout.com web is up on http://localhost:$COM_WEB_PORT"
    else
        warn "JobShout.com web is not responding on port $COM_WEB_PORT"
    fi
    if curl -sf "http://localhost:$COM_API_PORT/health" >/dev/null 2>&1; then
        say "JobShout.com API is up on http://localhost:$COM_API_PORT"
    else
        warn "JobShout.com API is not responding on port $COM_API_PORT"
    fi
    exit 0
fi

if [[ "$ACTION" == "logs" ]]; then
    info "Tailing logs. Ctrl+C to stop watching (services keep running)."
    tail -n 100 -F "$LOG_DIR/server.log" "$LOG_DIR/ui.log" "$LOG_DIR/com-web.log" 2>/dev/null
    exit 0
fi

# ============================================
# Main Development Flow
# ============================================
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}   JobShout Development Environment     ${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""

if [[ "${EUID}" -eq 0 ]]; then
    fail "Refusing to run as root — this leaves root-owned build files in the"
    fail "repo that break every later run. Run it as yourself: ./start-local.sh"
    exit 1
fi

# ============================================
# Step 1: Check Prerequisites
# ============================================
say "Checking prerequisites..."
missing=0

command -v go >/dev/null 2>&1 || { fail "Go is not installed."; missing=1; }
command -v npm >/dev/null 2>&1 || { fail "npm is not installed."; missing=1; }

if ! $NO_INFRA; then
    if ! command -v docker >/dev/null 2>&1; then
        fail "Docker is not installed. Install from https://docs.docker.com/get-docker/"
        missing=1
    elif ! docker info >/dev/null 2>&1; then
        fail "Docker daemon is not running. Start Docker Desktop."
        missing=1
    fi
fi

[[ $missing -eq 1 ]] && exit 1
say "All prerequisites met"
echo ""

# ============================================
# Step 2: Load .env
# ============================================
if [[ -f "$ENV_FILE" ]]; then
    set -a
    # shellcheck disable=SC1091
    source "$ENV_FILE"
    set +a
    say "Loaded .env"
else
    warn "No .env found — model gateway auth and DB settings will use defaults."
fi
echo ""

# ============================================
# Step 3: Infra (Postgres + pgvector, MinIO)
# ============================================
if ! $NO_INFRA && ! $COM_ONLY; then
    if $RESET; then
        warn "Resetting the database (removing volumes)..."
        docker compose -f "$COMPOSE_FILE" down -v >/dev/null 2>&1
    fi
    say "Starting infra (postgres, minio)..."
    if ! docker compose -f "$COMPOSE_FILE" up -d postgres minio >/dev/null 2>&1; then
        fail "Could not start compose services. Try: docker compose up -d postgres minio"
        exit 1
    fi

    say "Waiting for Postgres..."
    attempt=0
    while [[ $attempt -lt 30 ]]; do
        if command -v pg_isready >/dev/null 2>&1; then
            pg_isready -q 2>/dev/null && break
        else
            docker compose -f "$COMPOSE_FILE" exec -T postgres pg_isready -q >/dev/null 2>&1 && break
        fi
        attempt=$((attempt + 1)); printf "."; sleep 1
    done
    echo ""
    if [[ $attempt -eq 30 ]]; then
        fail "Postgres did not become ready. Check: docker compose ps"
        exit 1
    fi
    say "Postgres is ready on localhost:$PG_PORT"
elif $COM_ONLY; then
    info "Skipping platform infra (--com-only)"
else
    info "Skipping infra (--no-infra)"
fi
echo ""

# ============================================
# Step 4: Preflight — ports and build permissions
# ============================================
say "Preflight..."
if ! $COM_ONLY; then
    free_port "$API_PORT" "API"
    free_port "$UI_PORT" "UI"
fi
$NO_COM || free_port "$COM_WEB_PORT" "JobShout.com web"

# Next refuses to start when it cannot delete its own previous output, which is
# exactly what a past sudo run causes.
if [[ -n "$(find "$WEB_DIR"/.next* -user root 2>/dev/null | head -1)" ]]; then
    warn "Found root-owned files in web/nextjs/.next (left by a sudo run)."
    info "Asking for your password to remove them."
    sudo rm -rf "$WEB_DIR"/.next "$WEB_DIR"/.next.root-owned-DELETE-ME "$WEB_DIR"/.next.root-leftovers-* 2>/dev/null
    if [[ -n "$(find "$WEB_DIR"/.next* -user root 2>/dev/null | head -1)" ]]; then
        # Carrying on just moves the failure to a Next stack trace about
        # unlinking app-paths-manifest.json, which explains nothing.
        fail "Could not remove them, so the UI would fail to start. Run:"
        fail "    sudo rm -rf $WEB_DIR/.next*"
        fail "then re-run this script."
        exit 1
    fi
fi
say "Ports free, build directory writable"
echo ""

# ============================================
# Step 5: Build the API
# ============================================
if ! $COM_ONLY; then
    say "Building the Go API..."
    if ! (cd "$SERVER_DIR" && go build -o bin/jobshout-server ./cmd/server); then
        fail "Build failed."
        exit 1
    fi
    say "Built server/bin/jobshout-server"
    echo ""
fi

# ============================================
# Step 6: Start services
# ============================================
if ! $COM_ONLY; then
say "Starting API on :$API_PORT  (logs: .dev-logs/server.log)"
(
    cd "$SERVER_DIR"
    DATABASE_URL="${DATABASE_URL:-postgres://jobshout:jobshout@localhost:$PG_PORT/jobshout?sslmode=disable}" \
    JWT_SECRET="${JWT_SECRET:-dev-only-change-me-to-a-random-32-character-string}" \
    SERVER_PORT="0.0.0.0:$API_PORT" \
    CORS_ORIGINS="${CORS_ORIGINS:-http://localhost:$UI_PORT}" \
    MINIO_ENDPOINT="${MINIO_ENDPOINT:-}" \
    PYTHON_SIDECAR_URL="${PYTHON_SIDECAR_URL:-}" \
    LANGFUSE_HOST="${LANGFUSE_HOST:-}" \
    LANGFUSE_PUBLIC_KEY="${LANGFUSE_PUBLIC_KEY:-}" \
    LANGFUSE_SECRET_KEY="${LANGFUSE_SECRET_KEY:-}" \
    ./bin/jobshout-server
) >"$LOG_DIR/server.log" 2>&1 &
echo $! >"$API_PID_FILE"

say "Starting UI on :$UI_PORT  (logs: .dev-logs/ui.log)"
(
    cd "$WEB_DIR"
    [[ -d node_modules ]] || npm install
    # PORT is what actually sets the port: package.json's dev script reads it.
    # It used to be hardcoded to 3001, so UI_PORT here silently did nothing.
    PORT="$UI_PORT" \
    NEXT_PUBLIC_API_URL="http://localhost:$API_PORT" \
    NEXT_PUBLIC_WS_URL="ws://localhost:$API_PORT" \
    npm run dev
) >"$LOG_DIR/ui.log" 2>&1 &
echo $! >"$UI_PID_FILE"
fi

# ── JobShout.com ────────────────────────────────────────────────────────────
# Its own compose brings up the Rust API plus that product's Postgres, Redis,
# NATS and MinIO. The web front end runs natively so it hot-reloads like the
# platform UI does.
if ! $NO_COM; then
    echo ""
    if [[ -f "$COM_COMPOSE_FILE" ]] && ! $NO_INFRA; then
        say "Starting JobShout.com services (api, postgres, redis, nats, minio)..."
        if ! docker compose -f "$COM_COMPOSE_FILE" up -d postgres redis nats minio api >/dev/null 2>&1; then
            warn "Could not start the JobShout.com services — its web will run without an API."
        fi
    fi

    if [[ -d "$COM_WEB_DIR" ]]; then
        say "Starting JobShout.com web on :$COM_WEB_PORT  (logs: .dev-logs/com-web.log)"
        (
            cd "$COM_WEB_DIR"
            [[ -d node_modules ]] || npm install
            # NextAuth warns (and misbehaves on callbacks) without these.
            PORT="$COM_WEB_PORT" \
            JOBSHOUT_COM_API_URL="http://127.0.0.1:$COM_API_PORT" \
            NEXT_PUBLIC_JOBSHOUT_COM_API_URL="http://localhost:$COM_API_PORT" \
            NEXTAUTH_URL="${NEXTAUTH_URL:-http://localhost:$COM_WEB_PORT}" \
            NEXTAUTH_SECRET="${NEXTAUTH_SECRET:-dev-only-jobshout-com-secret-change-me}" \
            npm run dev
        ) >"$LOG_DIR/com-web.log" 2>&1 &
        echo $! >"$COM_WEB_PID_FILE"
    else
        warn "jobshout-com/web/nextjs not found — skipping."
    fi
fi

# ============================================
# Step 7: Wait for health
# ============================================
echo ""
say "Waiting for services..."
attempt=0
while [[ $attempt -lt 45 ]]; do
    ready=true
    if ! $COM_ONLY; then
        curl -sf "http://localhost:$API_PORT/health" >/dev/null 2>&1 || ready=false
        curl -sf "http://localhost:$UI_PORT/" >/dev/null 2>&1 || ready=false
    fi
    if ! $NO_COM; then
        curl -sf "http://localhost:$COM_WEB_PORT/" >/dev/null 2>&1 || ready=false
    fi
    if $ready; then
        say "Services are up!"
        break
    fi
    attempt=$((attempt + 1)); printf "."; sleep 2
done
echo ""
if [[ $attempt -eq 45 ]]; then
    warn "Services may not be fully ready — the first Next.js compile is slow."
    warn "Check .dev-logs/ui.log and .dev-logs/server.log"
fi

# ============================================
# Step 8: Print URLs
# ============================================
echo ""
echo -e "${CYAN}========================================${NC}"
echo -e "${CYAN}   JobShout is running!                 ${NC}"
echo -e "${CYAN}========================================${NC}"
echo ""
if ! $COM_ONLY; then
echo -e "  ${CYAN}Platform${NC}"
echo -e "    UI:            ${GREEN}http://localhost:$UI_PORT${NC}"
echo -e "    API:           ${GREEN}http://localhost:$API_PORT${NC}"
echo -e "    API health:    ${GREEN}http://localhost:$API_PORT/health${NC}"
echo -e "    MinIO console: ${GREEN}http://localhost:$MINIO_CONSOLE_PORT${NC}  (minioadmin / minioadmin)"
echo -e "    PostgreSQL:    ${GREEN}localhost:$PG_PORT${NC}  (jobshout / jobshout)"
echo ""
fi
if ! $NO_COM; then
echo -e "  ${CYAN}JobShout.com${NC}"
echo -e "    Web:           ${GREEN}http://localhost:$COM_WEB_PORT${NC}"
echo -e "    API:           ${GREEN}http://localhost:$COM_API_PORT${NC}"
echo -e "    MinIO console: ${GREEN}http://localhost:$COM_MINIO_CONSOLE_PORT${NC}"
echo -e "    PostgreSQL:    ${GREEN}localhost:$COM_PG_PORT${NC}"
echo ""
fi
echo -e "${CYAN}  Hot-Reload (no restart needed):${NC}"
echo -e "  web/nextjs/**            → Next.js Fast Refresh updates on save"
echo -e "  jobshout-com/web/**      → Next.js Fast Refresh updates on save"
echo -e "  public/landing/**        → static, just refresh the browser"
echo -e "  server/** (Go)           → ${YELLOW}needs a restart${NC} (compiled binary)"
echo -e "  .env                     → ${YELLOW}needs a restart${NC} (read once at boot)"
echo ""
echo -e "${CYAN}  Quick Commands:${NC}"
echo -e "  Stop:     ${YELLOW}./start-local.sh --stop${NC}"
echo -e "  Status:   ${YELLOW}./start-local.sh --status${NC}"
echo -e "  Logs:     ${YELLOW}./start-local.sh --logs${NC}"
echo -e "  Clean:    ${YELLOW}./start-local.sh --clean${NC}  (wipes the database)"
echo ""
echo -e "${CYAN}========================================${NC}"
echo ""
info "If the styling looks stale, hard-refresh the browser (Cmd+Shift+R)."
echo ""

cleanup() {
    echo ""
    say "Shutting down..."
    stop_services
    say "Done. Goodbye!"
    exit 0
}

if $DETACH; then
    say "Running in the background. Use ./start-local.sh --stop to stop."
    exit 0
fi

trap cleanup SIGINT SIGTERM

info "Attaching to logs... Press Ctrl+C to stop everything."
echo ""
TAIL_LOGS=()
$COM_ONLY || TAIL_LOGS+=("$LOG_DIR/server.log" "$LOG_DIR/ui.log")
$NO_COM  || TAIL_LOGS+=("$LOG_DIR/com-web.log")
tail -n +1 -F "${TAIL_LOGS[@]}"
