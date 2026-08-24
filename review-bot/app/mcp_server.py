"""Standalone MCP server (Streamable HTTP) for the Mac launchd service.

Editors call `start_review` (returns a job id in under a second), then poll
`review_status` until the job reaches a terminal state. Cursor's tool-call
timeout is fixed at ~60s and progress notifications do not extend it, so no
tool here may ever block on a review.

The tool bodies and the MCPServer factory live in app.mcp_app so the in-cluster
http_server can mount the exact same tools. This module keeps the Mac-specific
bits: launchd env, hostname-based DNS-rebinding allowlist, and its own uvicorn.
"""

from __future__ import annotations

import os
import socket
from urllib.parse import urlparse

from dotenv import load_dotenv
from mcp.server.mcpserver.exceptions import ToolError
from mcp.server.transport_security import TransportSecuritySettings

from app.config.settings import ConfigError, load_settings
from app.jobs import JobRunner
from app.mcp_app import build_mcp, do_list_repos, do_prime, do_review_status, do_start_review
from app.repos import RepoRegistry

DEFAULT_PORT = 8765

load_dotenv()  # same .env the CLI/watcher use; launchd's WorkingDirectory makes this resolve

_PORT = int(os.getenv("REVIEW_BOT_PORT", str(DEFAULT_PORT)))
_HOSTNAME_RAW = socket.gethostname()  # e.g. "Balinders-Mac-Studio.local" — case preserved
_HOSTNAME = _HOSTNAME_RAW.lower()
_PUBLIC_URL = os.getenv("REVIEW_BOT_URL", f"http://{_HOSTNAME}:{_PORT}").rstrip("/")

# Populated by main() before the server starts accepting requests. The tests set
# these directly and call the module-level tool wrappers below.
_runner: JobRunner | None = None
_registry: RepoRegistry | None = None


def _require_runner() -> tuple[JobRunner, RepoRegistry]:
    if _runner is None or _registry is None:
        raise ToolError("Server is still starting up — try again in a moment.")
    return _runner, _registry


def start_review(repo: str, pr_number: int, dry_run: bool = False, force: bool = False) -> dict:
    runner, _ = _require_runner()
    return do_start_review(runner, repo, pr_number, dry_run=dry_run, force=force)


def review_status(job_id: str) -> dict:
    runner, _ = _require_runner()
    return do_review_status(runner, job_id)


def prime(repo: str, force: bool = False) -> dict:
    runner, _ = _require_runner()
    return do_prime(runner, repo, force=force)


def list_repos() -> dict:
    _, registry = _require_runner()
    return do_list_repos(registry)


def _allowed_hosts() -> list[str]:
    """Host-header values clients may use to reach us (DNS-rebinding guard).

    The SDK middleware matches Host headers case-SENSITIVELY, but hostnames are
    case-insensitive on the wire (curl preserves the case you type; browsers and
    Node lowercase it) — so every name goes in with its original case AND lowered.
    """
    names = {"localhost", "127.0.0.1"}
    for base in (_HOSTNAME_RAW, urlparse(_PUBLIC_URL).hostname or ""):
        for variant in (base, base.lower()):
            if not variant:
                continue
            short = variant.split(".")[0]
            names.update({variant, short, f"{short}.local"})
    try:
        # Route lookup only — no packet is sent; yields the primary LAN address.
        probe = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        probe.connect(("8.8.8.8", 80))
        names.add(probe.getsockname()[0])
        probe.close()
    except OSError:
        pass
    for extra in os.getenv("REVIEW_BOT_ALLOWED_HOSTS", "").split(","):
        if extra.strip():
            names.update({extra.strip(), extra.strip().lower()})
    return sorted(f"{name}:*" for name in names)


def main() -> None:
    global _runner, _registry
    if not os.getenv("REVIEW_BOT_TOKEN", "").strip():
        raise ConfigError("REVIEW_BOT_TOKEN is not set. Add it to .env — generate one with: openssl rand -hex 32")

    settings = load_settings()
    _registry = RepoRegistry.load(settings)
    _runner = JobRunner(settings, _registry)

    mcp = build_mcp(_runner, _registry, _PUBLIC_URL)
    mcp.run(
        transport="streamable-http",
        host="0.0.0.0",
        port=_PORT,
        streamable_http_path="/mcp",
        transport_security=TransportSecuritySettings(allowed_hosts=_allowed_hosts()),
    )


if __name__ == "__main__":
    main()
