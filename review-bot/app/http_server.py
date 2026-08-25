"""Cluster HTTP API for JobShout: start/poll reviews over JSON, plus MCP.

This process is what k3s runs. It serves two things off the same JobRunner (one
review queue):
  - REST on REVIEW_BOT_PORT (8765): /health for probes, /api/* behind a shared
    bearer token. The JobShout reconciler talks to this, cluster-internal only.
  - MCP (Streamable HTTP) on REVIEW_BOT_MCP_PORT (8766): the same four tools as
    the Mac's mcp_server, for editor CLIs (Claude Code / Cursor). nginx exposes
    only /mcp publicly; the REST port stays internal.

Both share ONE JobRunner on purpose: two runners would race on the git-worktree
prune (see jobs.py). MCP runs in a background thread; uvicorn skips signal
handlers off the main thread, so mcp.run() there is safe.

OpenCode talks to a loopback JWT proxy so Ollama gateway tokens do not expire
mid-review. Watch is not started.
"""

from __future__ import annotations

import json
import os
import shutil
import threading
from dataclasses import replace
from urllib.parse import urlparse

from dotenv import load_dotenv
from mcp.server.transport_security import TransportSecuritySettings
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Route

from app.config.settings import ConfigError, Settings, load_settings
from app.jobs import JobRunner
from app.mcp_app import build_mcp
from app.ollama_proxy import proxy_base_url, start_proxy
from app.repos import RepoNotAllowed, RepoRegistry

DEFAULT_PORT = 8765
DEFAULT_MCP_PORT = 8766
PROXY_LISTEN = ("127.0.0.1", 11434)

load_dotenv()

_runner: JobRunner | None = None
_registry: RepoRegistry | None = None
_settings: Settings | None = None
_token = ""


def _unauthorized() -> JSONResponse:
    return JSONResponse({"error": "unauthorized"}, status_code=401)


def _require_auth(request: Request) -> JSONResponse | None:
    if request.url.path == "/health":
        return None
    expected = _token
    if not expected:
        return JSONResponse({"error": "REVIEW_BOT_TOKEN is not set"}, status_code=500)
    header = request.headers.get("authorization", "")
    prefix = "Bearer "
    if not header.startswith(prefix) or header[len(prefix) :] != expected:
        return _unauthorized()
    return None


async def health(_request: Request) -> Response:
    """Process up only. Do not probe Ollama, git, or OpenCode."""
    return JSONResponse({"status": "ok"})


async def capabilities(request: Request) -> JSONResponse:
    if err := _require_auth(request):
        return err
    assert _registry is not None and _settings is not None
    clones = _registry.clones_dir
    writable = False
    try:
        clones.mkdir(parents=True, exist_ok=True)
        probe = clones / ".write-probe"
        probe.write_text("ok")
        probe.unlink(missing_ok=True)
        writable = True
    except OSError:
        writable = False
    return JSONResponse(
        {
            "opencode": {"available": shutil.which("opencode") is not None},
            "clones_dir": {"path": str(clones), "writable": writable},
            "ollama": {
                "open_code_host": _settings.ollama_host,
                "model": _settings.model,
                "jwt_proxy": (urlparse(_settings.ollama_host).hostname or "").lower()
                in {"127.0.0.1", "localhost"},
            },
            "allowed": _registry.allowed_slugs(),
        }
    )


async def list_repos(request: Request) -> JSONResponse:
    if err := _require_auth(request):
        return err
    assert _registry is not None
    return JSONResponse({"allowed": _registry.allowed_slugs()})


async def start_review(request: Request) -> JSONResponse:
    if err := _require_auth(request):
        return err
    assert _runner is not None
    try:
        payload = await request.json()
    except json.JSONDecodeError:
        return JSONResponse({"error": "invalid JSON body"}, status_code=400)
    if not isinstance(payload, dict):
        return JSONResponse({"error": "JSON object required"}, status_code=400)

    repo = payload.get("repo")
    if not isinstance(repo, str) or not repo.strip():
        return JSONResponse({"error": "repo is required (owner/name)"}, status_code=400)
    try:
        pr_number = int(payload.get("pr_number"))
    except (TypeError, ValueError):
        return JSONResponse({"error": "pr_number must be an integer"}, status_code=400)

    # Posts to GitHub by default; callers opt into a preview with dry_run=true.
    dry_run = bool(payload.get("dry_run", False))
    force = bool(payload.get("force", False))
    run_ref = payload.get("run_ref") or ""
    if run_ref and not isinstance(run_ref, str):
        return JSONResponse({"error": "run_ref must be a string"}, status_code=400)
    run_ref = str(run_ref)

    prior = _runner.find_by_run_ref(run_ref) if run_ref else None
    try:
        job = _runner.submit_review(
            repo.strip(), pr_number, dry_run=dry_run, force=force, run_ref=run_ref
        )
    except RepoNotAllowed as exc:
        return JSONResponse({"error": str(exc)}, status_code=403)

    existing = prior is not None and prior.id == job.id
    position = _runner.queue_position(job)
    body = {
        "job_id": job.id,
        "state": job.state,
        "queue_position": position if position is not None else 0,
        "existing": existing,
    }
    return JSONResponse(body, status_code=200 if existing else 202)


async def review_status(request: Request) -> JSONResponse:
    if err := _require_auth(request):
        return err
    assert _runner is not None
    job_id = request.path_params["job_id"]
    snapshot = _runner.snapshot(job_id)
    if snapshot is None:
        return JSONResponse(
            {
                "error": (
                    f"Job {job_id!r} not found — the server may have restarted. "
                    "Submit the review again."
                )
            },
            status_code=404,
        )
    return JSONResponse(snapshot)


def create_app(runner: JobRunner, registry: RepoRegistry, settings: Settings, token: str) -> Starlette:
    """Testable app factory. Production main() wires this after load_settings."""
    global _runner, _registry, _settings, _token
    _runner = runner
    _registry = registry
    _settings = settings
    _token = token
    return Starlette(
        routes=[
            Route("/health", health, methods=["GET"]),
            Route("/api/capabilities", capabilities, methods=["GET"]),
            Route("/api/repos", list_repos, methods=["GET"]),
            Route("/api/reviews", start_review, methods=["POST"]),
            Route("/api/reviews/{job_id}", review_status, methods=["GET"]),
        ]
    )


def _needs_jwt_proxy(upstream: str, secret: str) -> bool:
    if not secret:
        return False
    host = (urlparse(upstream).hostname or "").lower()
    return host not in {"127.0.0.1", "localhost"}


def _mcp_allowed_hosts() -> list[str]:
    """Host-header values the MCP DNS-rebinding guard accepts.

    Requests arrive via nginx with Host set to the public ingress host, so that
    name (from REVIEW_BOT_ALLOWED_HOSTS) must be present or MCP returns 421. The
    SDK matches case-sensitively, so add each name lowered too.
    """
    names = {"localhost", "127.0.0.1", "jobshout-review-bot"}
    for raw in os.getenv("REVIEW_BOT_ALLOWED_HOSTS", "").split(","):
        name = raw.strip()
        if name:
            names.update({name, name.lower()})
    # nginx forwards a port-less Host, so the bare name must be an exact allow;
    # the ":*" form additionally covers host:port (local curl, port-forward).
    hosts: set[str] = set()
    for name in names:
        hosts.update({name, f"{name}:*"})
    return sorted(hosts)


def _start_mcp(runner: JobRunner, registry: RepoRegistry) -> None:
    """Serve the MCP tools in a daemon thread on REVIEW_BOT_MCP_PORT, sharing runner.

    The public URL (for the MCP auth metadata) is REVIEW_BOT_PUBLIC_URL when set,
    else the internal REVIEW_BOT_URL. mcp.run() runs its own uvicorn; off the main
    thread uvicorn skips signal handlers, so this does not fight the REST server.
    """
    mcp_port = int(os.getenv("REVIEW_BOT_MCP_PORT", str(DEFAULT_MCP_PORT)))
    public_url = (os.getenv("REVIEW_BOT_PUBLIC_URL", "").strip() or os.getenv("REVIEW_BOT_URL", "").strip()).rstrip("/")
    mcp = build_mcp(runner, registry, public_url or f"http://localhost:{mcp_port}")
    allowed = _mcp_allowed_hosts()

    def run() -> None:
        mcp.run(
            transport="streamable-http",
            host="0.0.0.0",
            port=mcp_port,
            streamable_http_path="/mcp",
            transport_security=TransportSecuritySettings(allowed_hosts=allowed),
        )

    threading.Thread(target=run, name="review-mcp", daemon=True).start()


def main() -> None:
    global _runner, _registry, _settings, _token
    token = os.getenv("REVIEW_BOT_TOKEN", "").strip()
    if not token:
        raise ConfigError(
            "REVIEW_BOT_TOKEN is not set. Add it to the environment — generate one with: openssl rand -hex 32"
        )

    settings = load_settings()
    registry = RepoRegistry.load(settings)
    if not registry.allowed_slugs():
        raise ConfigError(
            "No repos allowed. Set LOCAL_REPO_PATH to a clone, or provide repos.json with an allowlist."
        )

    secret = os.getenv("OLLAMA_JWT_SECRET", "").strip()
    if _needs_jwt_proxy(settings.ollama_host, secret):
        start_proxy(settings.ollama_host, secret, PROXY_LISTEN)
        settings = replace(settings, ollama_host=proxy_base_url(PROXY_LISTEN))
        registry = RepoRegistry(settings, allowed=set(registry.allowed), clones_dir=registry.clones_dir)

    runner = JobRunner(settings, registry)
    app = create_app(runner, registry, settings, token)
    port = int(os.getenv("REVIEW_BOT_PORT", str(DEFAULT_PORT)))

    # Same runner (one review queue) served over MCP for editor CLIs.
    _start_mcp(runner, registry)

    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=port, log_level="info")


if __name__ == "__main__":
    main()
