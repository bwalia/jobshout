# Pin TARGETPLATFORM so buildx on an arm64 Mac cannot tag an arm64 image
# as linux/amd64 (same class of bug as jobshout-web / jobshout-server).
FROM --platform=$TARGETPLATFORM python:3.12-slim
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "amd64" ]; then uname -m | grep -qx x86_64; fi

# Heavy amd64 (QEMU) install — cache mounts + CI registry/local layer cache are
# what keep this under the deploy job timeout on the arm64 Mac runner.
RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    --mount=type=cache,target=/root/.npm \
    apt-get update && apt-get install -y --no-install-recommends \
        git ca-certificates curl gnupg \
    && curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && npm install -g opencode-ai \
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
