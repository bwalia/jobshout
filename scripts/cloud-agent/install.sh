#!/usr/bin/env bash
#
# Cloud Agent install phase — idempotent dependency refresh after checkout.
#
# System toolchains (Go 1.25, PostgreSQL 16 + pgvector, Node) are provided by
# the environment's base snapshot; this script only prepares repository state
# that is derived from the checked-out source, so it is safe to run repeatedly.
set -euo pipefail

export PATH="/usr/local/go/bin:$PATH"

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

echo "[install] go:   $(go version)"
echo "[install] node: $(node --version)"

echo "[install] downloading Go modules"
( cd server && go mod download )

# Precompile the server so the first API boot is fast and any build break
# surfaces here rather than at runtime.
echo "[install] building Go server"
( cd server && go build -o bin/jobshout-server ./cmd/server )

echo "[install] installing web dependencies"
( cd web/nextjs && npm ci )

echo "[install] done"
