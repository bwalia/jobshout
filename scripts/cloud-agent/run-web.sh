#!/usr/bin/env bash
#
# Cloud Agent terminal — the Next.js dev server on :3001, pointed at the local
# Go API. The NEXT_PUBLIC_* URLs are read by the browser, so they use the
# host-reachable localhost addresses of the API terminal.
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

export NEXT_PUBLIC_API_URL="${NEXT_PUBLIC_API_URL:-http://localhost:8080}"
export NEXT_PUBLIC_WS_URL="${NEXT_PUBLIC_WS_URL:-ws://localhost:8080}"

cd "$ROOT_DIR/web/nextjs"
exec npm run dev
