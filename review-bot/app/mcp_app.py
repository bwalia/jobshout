"""Shared MCP definition: the four review tools, the token verifier, and a
factory that binds them to a JobRunner.

Both entrypoints use this so there is exactly one tool implementation:
  - mcp_server.py  — the standalone Mac launchd service (its own uvicorn).
  - http_server.py — the in-cluster server, which mounts build_mcp()'s ASGI app
    at /mcp alongside the REST API, sharing the SAME JobRunner (one review queue).
"""

from __future__ import annotations

import os
import secrets

from mcp.server import MCPServer
from mcp.server.auth.provider import AccessToken, TokenVerifier
from mcp.server.auth.settings import AuthSettings
from mcp.server.mcpserver.exceptions import ToolError

from app.jobs import JobRunner
from app.repos import RepoNotAllowed, RepoRegistry

INSTRUCTIONS = (
    "AI PR reviewer running on a central machine. Call start_review(repo, "
    "pr_number) to queue a review — it returns a job_id immediately. Then poll "
    "review_status(job_id) about every 30 seconds until state is 'done' or "
    "'failed'; a review takes a few minutes. list_repos shows the allowed repos."
)


class StaticTokenVerifier(TokenVerifier):
    """Accepts exactly the shared token from REVIEW_BOT_TOKEN."""

    async def verify_token(self, token: str) -> AccessToken | None:
        expected = os.getenv("REVIEW_BOT_TOKEN", "").strip()
        if expected and secrets.compare_digest(token, expected):
            return AccessToken(token=token, client_id="review-bot-client", scopes=[])
        return None


# -- tool bodies (single source of truth for both entrypoints) --------------


def do_start_review(
    runner: JobRunner, repo: str, pr_number: int, dry_run: bool = False, force: bool = False
) -> dict:
    try:
        job = runner.submit_review(repo, pr_number, dry_run=dry_run, force=force)
    except RepoNotAllowed as exc:
        raise ToolError(str(exc)) from exc
    position = runner.queue_position(job)
    return {"job_id": job.id, "state": job.state, "queue_position": position if position is not None else 0}


def do_review_status(runner: JobRunner, job_id: str) -> dict:
    snapshot = runner.snapshot(job_id)
    if snapshot is None:
        raise ToolError(f"Job {job_id!r} not found — the server may have restarted. Submit the review again.")
    return snapshot


def do_prime(runner: JobRunner, repo: str, force: bool = False) -> dict:
    try:
        job = runner.submit_prime(repo, force=force)
    except RepoNotAllowed as exc:
        raise ToolError(str(exc)) from exc
    position = runner.queue_position(job)
    return {"job_id": job.id, "state": job.state, "queue_position": position if position is not None else 0}


def do_list_repos(registry: RepoRegistry) -> dict:
    return {"allowed": registry.allowed_slugs()}


def build_mcp(runner: JobRunner, registry: RepoRegistry, public_url: str) -> MCPServer:
    """An MCPServer with the four review tools bound to this runner/registry."""
    public_url = public_url.rstrip("/")
    mcp = MCPServer(
        "review-bot",
        instructions=INSTRUCTIONS,
        token_verifier=StaticTokenVerifier(),
        auth=AuthSettings(
            issuer_url=public_url,
            resource_server_url=f"{public_url}/mcp",
        ),
    )

    @mcp.tool()
    def start_review(repo: str, pr_number: int, dry_run: bool = False, force: bool = False) -> dict:
        """Queue an AI review of a GitHub pull request; returns a job_id immediately.

        repo is "owner/name". Poll review_status(job_id) until done or failed.
        dry_run=true produces findings without posting anything to GitHub.
        force=true re-reviews a commit even if it was already reviewed.
        """
        return do_start_review(runner, repo, pr_number, dry_run=dry_run, force=force)

    @mcp.tool()
    def review_status(job_id: str) -> dict:
        """Status of a review/prime job: state (queued|running|done|failed), stage_log,
        queue_position while queued, the serialized review in result when done, and
        error when failed."""
        return do_review_status(runner, job_id)

    @mcp.tool()
    def prime(repo: str, force: bool = False) -> dict:
        """Queue a one-time deep exploration of a repo to build its cached repo map
        (improves review quality). Same job API: poll review_status(job_id).
        force=true rebuilds an existing map."""
        return do_prime(runner, repo, force=force)

    @mcp.tool()
    def list_repos() -> dict:
        """The repos this server may review (allowlist from repos.json plus the
        watcher's own repo)."""
        return do_list_repos(registry)

    return mcp
