# Pin the builder to the NATIVE build platform so the Go toolchain runs
# un-emulated (the amd64 Go compiler segfaults under QEMU on an arm64 host).
# Go then cross-compiles to the target arch via GOARCH below.
FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy host CA certs for corporate proxy/VPN TLS interception
COPY certs/ /usr/local/share/ca-certificates/
RUN update-ca-certificates || true

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Release tag (v1.0.8). CI passes --build-arg VERSION=$NEXT_VERSION so /health
# and the sidebar show the git tag that built this image, not a hardcoded string.
ARG VERSION=dev
# TARGETOS/TARGETARCH are injected by buildx from --platform (e.g. linux/amd64).
# When building without buildx (local docker compose), they are unset and Go
# builds for the native arch — so local builds stay correct.
ARG TARGETOS
ARG TARGETARCH
# TARGETOS/TARGETARCH are injected by buildx from --platform (e.g. linux/amd64).
# Local compose on an arm64 Mac sets TARGETARCH=arm64 — that is correct there.
# CI always builds --platform linux/amd64.
RUN echo "building GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)}"
# GOARCH/GOOS must be literal assignment tokens (the shell recognizes
# assignments before expansion, so ${VAR:+GOARCH=...} would be parsed as a
# command). Default to the native arch when TARGETARCH is unset (local builds).
RUN CGO_ENABLED=0 GOOS="${TARGETOS:-linux}" GOARCH="${TARGETARCH:-$(go env GOARCH)}" \
    go build -ldflags="-s -w -X main.version=${VERSION}" -o /jobshout-server ./cmd/server

# Pin the runtime base to TARGETPLATFORM. Without it, buildx on an arm64 Mac
# has shipped an arm64 alpine tagged as linux/amd64; k3s then CrashLoops with
# `exec ./jobshout-server: no such file or directory`.
FROM --platform=$TARGETPLATFORM alpine:3.19
ARG TARGETARCH
RUN if [ "$TARGETARCH" = "amd64" ]; then uname -m | grep -qx x86_64; fi

# git is required at runtime by the automated blog generator, which shells
# out to `git clone/commit/push` against the target repo.
RUN apk add --no-cache ca-certificates tzdata git

WORKDIR /app

COPY --from=builder /jobshout-server .
COPY --from=builder /app/migrations ./migrations

EXPOSE 8080

CMD ["./jobshout-server"]
