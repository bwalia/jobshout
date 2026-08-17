#!/usr/bin/env bash
# Run the image service against the mflux tool environment.
#
# It borrows uv's mflux environment rather than building its own: mflux pulls in
# MLX and several gigabytes of model plumbing, and a second copy of that in a
# private virtualenv would cost the disk twice over for no benefit. Only the
# packages that environment lacks — FastAPI, uvicorn, PyJWT — get installed.
set -euo pipefail

cd "$(dirname "$0")"

MFLUX_ENV="${MFLUX_ENV:-$HOME/.local/share/uv/tools/mflux}"
PYTHON="$MFLUX_ENV/bin/python"

if [[ ! -x "$PYTHON" ]]; then
  echo "mflux environment not found at $MFLUX_ENV" >&2
  echo "Install it with:  uv tool install mflux" >&2
  echo "Or point MFLUX_ENV at an existing install." >&2
  exit 1
fi

# Idempotent: skipped entirely once the imports resolve, so this costs nothing on
# a warm machine and removes a setup step from a new one.
#
# Installed with `uv pip --python`, not `python -m pip`: a uv tool environment
# ships no pip at all, so the obvious invocation fails with "No module named
# pip" on a machine that is otherwise perfectly set up.
if ! "$PYTHON" -c "import fastapi, uvicorn, jwt" >/dev/null 2>&1; then
  echo "Installing the service's own dependencies into the mflux environment…"
  if ! command -v uv >/dev/null 2>&1; then
    echo "uv is required to install into the mflux tool environment." >&2
    echo "Install it from https://docs.astral.sh/uv/ and re-run." >&2
    exit 1
  fi
  uv pip install --quiet --python "$PYTHON" -r requirements.txt
fi

export IMAGE_HOST="${IMAGE_HOST:-0.0.0.0}"
export IMAGE_PORT="${IMAGE_PORT:-11435}"

echo "jobshout-image-service listening on ${IMAGE_HOST}:${IMAGE_PORT}"
if [[ -z "${IMAGE_JWT_SECRET:-}" ]]; then
  echo "WARNING: IMAGE_JWT_SECRET is unset — every request will be accepted." >&2
fi

# One worker, always. The GPU lock is per-process, so a second worker would be a
# second process holding its own lock and its own resident model — which is
# precisely the concurrent generation the lock exists to prevent.
exec "$PYTHON" -m uvicorn app.main:app \
  --host "$IMAGE_HOST" \
  --port "$IMAGE_PORT" \
  --workers 1 \
  --log-level "${IMAGE_LOG_LEVEL:-info}"
