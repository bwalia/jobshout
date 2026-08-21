#!/usr/bin/env bash
# Run the pentest service.
#
# Unlike image-service, which borrows uv's multi-gigabyte mflux environment,
# this one owns a small virtualenv: Strix is a subprocess, not an import, so
# there is nothing large to share.
set -euo pipefail

cd "$(dirname "$0")"

VENV="${STRIX_SERVICE_VENV:-.venv}"
PYTHON="$VENV/bin/python"

if [[ ! -x "$PYTHON" ]]; then
  if ! command -v uv >/dev/null 2>&1; then
    echo "uv is required to create the service virtualenv." >&2
    echo "Install it from https://docs.astral.sh/uv/ and re-run." >&2
    exit 1
  fi
  echo "Creating the service virtualenv at $VENV…"
  uv venv "$VENV"
fi

# Idempotent: skipped entirely once the imports resolve, so this costs nothing
# on a warm machine and removes a setup step from a new one.
if ! "$PYTHON" -c "import fastapi, uvicorn, jwt" >/dev/null 2>&1; then
  echo "Installing dependencies…"
  uv pip install --quiet --python "$PYTHON" -r requirements.txt
fi

export STRIX_HOST="${STRIX_HOST:-0.0.0.0}"
export STRIX_PORT="${STRIX_PORT:-11436}"

echo "jobshout-pentest-service listening on ${STRIX_HOST}:${STRIX_PORT}"

if [[ -z "${STRIX_JWT_SECRET:-}" ]]; then
  echo "WARNING: STRIX_JWT_SECRET is unset — every request will be accepted." >&2
fi
if [[ -z "${STRIX_TARGET_ALLOWLIST:-}" ]]; then
  echo "WARNING: STRIX_TARGET_ALLOWLIST is unset — every scan will be REFUSED." >&2
  echo "         This is the safe default. Set it to the hosts you are authorised to test." >&2
fi
if ! docker info >/dev/null 2>&1; then
  echo "WARNING: Docker is not responding. Strix sandboxes each scan in containers," >&2
  echo "         so scans will fail until Docker Desktop is running." >&2
fi

# One worker, always. The concurrency limit and the run registry are both
# per-process, so a second worker would be a second registry answering for runs
# it has never heard of.
exec "$PYTHON" -m uvicorn app.main:app \
  --host "$STRIX_HOST" \
  --port "$STRIX_PORT" \
  --workers 1 \
  --log-level "${STRIX_LOG_LEVEL:-info}"
