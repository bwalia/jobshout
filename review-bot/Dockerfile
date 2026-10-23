# Pin TARGETPLATFORM so buildx on an arm64 Mac cannot tag an arm64 image
# as linux/amd64 (same class of bug as jobshout-web / jobshout-server).
FROM --platform=$TARGETPLATFORM python:3.12-slim
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "amd64" ]; then uname -m | grep -qx x86_64; fi

# Heavy amd64 (QEMU) install — cache mounts + CI registry/local layer cache are
# what keep this under the deploy job timeout on the arm64 Mac runner.
#
# opencode-ai is pinned: unpinned, it picked up 1.19.4 (2026-10-22), whose glibc
# binary npm dropped on that runner, so every image build — PR checks and master
# deploys — failed with EBADPLATFORM. 1.19.3 is the last version that built
# there. Bump deliberately and watch the PR image check.
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    --mount=type=cache,target=/root/.npm \
    apt-get update && apt-get install -y --no-install-recommends \
        git ca-certificates curl gnupg \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install -g opencode-ai@1.19.3 \
    && git config --global --add safe.directory '*' \
    && apt-get clean && rm -rf /var/lib/apt/lists/* \
    && git --version && opencode --version && uname -m

WORKDIR /app

COPY requirements.txt .
RUN --mount=type=cache,target=/root/.cache/pip \
    pip install --no-cache-dir -r requirements.txt

COPY app/ ./app/

ENV REVIEW_BOT_PORT=8765 \
    REVIEW_BOT_CLONES_DIR=/data/clones \
    REVIEW_BOT_CACHE_DIR=/data/cache \
    PYTHONUNBUFFERED=1

EXPOSE 8765

CMD ["python", "-m", "app.http_server"]
