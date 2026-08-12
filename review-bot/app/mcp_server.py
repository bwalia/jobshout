"""MCP server exposing review-bot over Streamable HTTP (start/poll job API).

Editors call `start_review` (returns a job id in under a second), then poll
`review_status` until the job reaches a terminal state. Cursor's tool-call
timeout is fixed at ~60s and progress notifications do not extend it, so no
tool here may ever block on a review.

Auth is a single shared bearer token (REVIEW_BOT_TOKEN in .env), checked by a
static TokenVerifier. The server binds 0.0.0.0 for LAN/Tailscale access, with
Host-header validation limited to this machine's names.
"""

from __future__ import annotations

import os
import secrets
import socket
from urllib.parse import urlparse

from dotenv import load_dotenv
from mcp.server import MCPServer
from mcp.server.auth.provider import AccessToken, TokenVerifier
from mcp.server.auth.settings import AuthSettings
from mcp.server.mcpserver.exceptions import ToolError
from mcp.server.transport_security import TransportSecuritySettings

from app.config.settings import ConfigError, load_settings
from app.jobs import JobRunner
from app.repos import RepoNotAllowed, RepoRegistry

DEFAULT_PORT = 8765

load_dotenv()  # same .env the CLI/watcher use; launchd's WorkingDirectory makes this resolve

_PORT = int(os.getenv("REVIEW_BOT_PORT", str(DEFAULT_PORT)))
_HOSTNAME = socket.gethostname().lower()
_PUBLIC_URL = os.getenv("REVIEW_BOT_URL", f"http://{_HOSTNAME}:{_PORT}").rstrip("/")


class StaticTokenVerifier(TokenVerifier):
    """Accepts exactly the shared token from REVIEW_BOT_TOKEN."""

    async def verify_token(self, token: str) -> AccessToken | None:
        expected = os.getenv("REVIEW_BOT_TOKEN", "").strip()
        if expected and secrets.compare_digest(token, expected):
            return AccessToken(token=token, client_id="review-bot-client", scopes=[])
        return None


mcp = MCPServer(
    "review-bot",
    instructions=(
        "AI PR reviewer running on a central machine. Call start_review(repo, "
        "pr_number) to queue a review — it returns a job_id immediately. Then poll "
        "review_status(job_id) about every 30 seconds until state is 'done' or "
        "'failed'; a review takes a few minutes. list_repos shows the allowed repos."
    ),
    token_verifier=StaticTokenVerifier(),
    auth=AuthSettings(
        issuer_url=_PUBLIC_URL,
        resource_server_url=f"{_PUBLIC_URL}/mcp",
    ),
)

# Populated by main() before the server starts accepting requests.
_runner: JobRunner | None = None
_registry: RepoRegistry | None = None


def _require_runner() -> tuple[JobRunner, RepoRegistry]:
    if _runner is None or _registry is None:
        raise ToolError("Server is still starting up — try again in a moment.")
    return _runner, _registry


@mcp.tool()
def start_review(repo: str, pr_number: int, dry_run: bool = False, force: bool = False) -> dict:
    """Queue an AI review of a GitHub pull request; returns a job_id immediately.

    repo is "owner/name". Poll review_status(job_id) until done or failed.
    dry_run=true produces findings without posting anything to GitHub.
    force=true re-reviews a commit even if it was already reviewed.
    """
    runner, _ = _require_runner()
    try:
        job = runner.submit_review(repo, pr_number, dry_run=dry_run, force=force)
    except RepoNotAllowed as exc:
        raise ToolError(str(exc)) from exc
    position = runner.queue_position(job)
    return {"job_id": job.id, "state": job.state, "queue_position": position if position is not None else 0}


@mcp.tool()
def review_status(job_id: str) -> dict:
    """Status of a review/prime job: state (queued|running|done|failed), stage_log,
    queue_position while queued, the serialized review in result when done, and
    error when failed."""
    runner, _ = _require_runner()
    snapshot = runner.snapshot(job_id)
    if snapshot is None:
        raise ToolError(f"Job {job_id!r} not found — the server may have restarted. Submit the review again.")
    return snapshot


@mcp.tool()
def prime(repo: str, force: bool = False) -> dict:
    """Queue a one-time deep exploration of a repo to build its cached repo map
    (improves review quality). Same job API: poll review_status(job_id).
    force=true rebuilds an existing map."""
    runner, _ = _require_runner()
    try:
        job = runner.submit_prime(repo, force=force)
    except RepoNotAllowed as exc:
        raise ToolError(str(exc)) from exc
    position = runner.queue_position(job)
    return {"job_id": job.id, "state": job.state, "queue_position": position if position is not None else 0}


@mcp.tool()
def list_repos() -> dict:
    """The repos this server may review (allowlist from repos.json plus the
    watcher's own repo)."""
    _, registry = _require_runner()
    return {"allowed": registry.allowed_slugs()}


def _allowed_hosts() -> list[str]:
    """Host-header values clients may use to reach us (DNS-rebinding guard)."""
    short = _HOSTNAME.split(".")[0]
    hosts = {"localhost:*", "127.0.0.1:*", f"{_HOSTNAME}:*", f"{short}:*", f"{short}.local:*"}
    public_host = urlparse(_PUBLIC_URL).hostname
    if public_host:
        hosts.add(f"{public_host}:*")
    try:
        # Route lookup only — no packet is sent; yields the primary LAN address.
        probe = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
        probe.connect(("8.8.8.8", 80))
        hosts.add(f"{probe.getsockname()[0]}:*")
        probe.close()
    except OSError:
        pass
    for extra in os.getenv("REVIEW_BOT_ALLOWED_HOSTS", "").split(","):
        if extra.strip():
            hosts.add(f"{extra.strip()}:*")
    return sorted(hosts)


def main() -> None:
    global _runner, _registry
    if not os.getenv("REVIEW_BOT_TOKEN", "").strip():
        raise ConfigError("REVIEW_BOT_TOKEN is not set. Add it to .env — generate one with: openssl rand -hex 32")

    settings = load_settings()
    _registry = RepoRegistry.load(settings)
    _runner = JobRunner(settings, _registry)

    mcp.run(
        transport="streamable-http",
        host="0.0.0.0",
        port=_PORT,
        streamable_http_path="/mcp",
        transport_security=TransportSecuritySettings(allowed_hosts=_allowed_hosts()),
    )


if __name__ == "__main__":
    main()
