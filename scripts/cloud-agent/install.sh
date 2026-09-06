#!/usr/bin/env bash
#
# Cloud Agent install phase — idempotent, self-contained environment bootstrap.
#
# Provisions the system toolchains JobShout needs that are not in the default
# base image (Go 1.25, PostgreSQL 16 + pgvector) and then prepares the
# repository (Go modules + server build, web dependencies). Everything is
# guarded so re-running is cheap and safe; with environment builds this runs
# once and is captured in the baseline snapshot.
set -euo pipefail

GO_VERSION="1.25.0"
PG_MAJOR="16"

log() { echo "[install] $*"; }

# ── Go 1.25 ──────────────────────────────────────────────────────────────────
install_go() {
  if [ -x /usr/local/go/bin/go ] && /usr/local/go/bin/go version | grep -q "go${GO_VERSION}"; then
    log "Go ${GO_VERSION} already installed"
  else
    log "installing Go ${GO_VERSION}"
    local tarball="go${GO_VERSION}.linux-amd64.tar.gz"
    curl -fsSL -o "/tmp/${tarball}" "https://go.dev/dl/${tarball}"
    sudo rm -rf /usr/local/go
    sudo tar -C /usr/local -xzf "/tmp/${tarball}"
    rm -f "/tmp/${tarball}"
  fi
  # Make Go discoverable for every shell / future boot.
  sudo ln -sf /usr/local/go/bin/go /usr/local/bin/go
  sudo ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt
  echo 'export PATH=$PATH:/usr/local/go/bin' | sudo tee /etc/profile.d/go.sh >/dev/null
  export PATH="/usr/local/go/bin:$PATH"
}

# ── PostgreSQL 16 + pgvector ─────────────────────────────────────────────────
install_postgres() {
  if [ -x "/usr/lib/postgresql/${PG_MAJOR}/bin/postgres" ] \
     && dpkg -s "postgresql-${PG_MAJOR}-pgvector" >/dev/null 2>&1; then
    log "PostgreSQL ${PG_MAJOR} + pgvector already installed"
    return
  fi
  log "installing PostgreSQL ${PG_MAJOR} + pgvector from PGDG"
  sudo apt-get install -y curl ca-certificates gnupg lsb-release >/dev/null
  sudo install -d /usr/share/postgresql-common/pgdg
  sudo curl -fsSL -o /usr/share/postgresql-common/pgdg/apt.postgresql.org.asc \
    https://www.postgresql.org/media/keys/ACCC4CF8.asc
  . /etc/os-release
  echo "deb [signed-by=/usr/share/postgresql-common/pgdg/apt.postgresql.org.asc] https://apt.postgresql.org/pub/repos/apt ${VERSION_CODENAME}-pgdg main" \
    | sudo tee /etc/apt/sources.list.d/pgdg.list >/dev/null
  sudo apt-get update >/dev/null
  sudo apt-get install -y "postgresql-${PG_MAJOR}" "postgresql-${PG_MAJOR}-pgvector" >/dev/null
}

# ── Repository dependencies ──────────────────────────────────────────────────
install_repo_deps() {
  local root_dir
  root_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
  cd "$root_dir"

  log "go:   $(go version)"
  log "node: $(node --version)"

  log "downloading Go modules and building the server"
  ( cd server && go mod download && go build -o bin/jobshout-server ./cmd/server )

  log "installing web dependencies"
  ( cd web/nextjs && npm ci )
}

install_go
install_postgres
install_repo_deps

log "done"
