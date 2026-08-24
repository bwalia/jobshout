"""Cluster HTTP API for JobShout: start/poll reviews over JSON.

Keep mcp_server.py for the Mac. This process is what k3s runs: /health for
probes, /api/* behind a shared bearer token, same JobRunner as MCP.

OpenCode talks to a loopback JWT proxy so Ollama gateway tokens do not expire
mid-review. Watch is not started.
"""

from __future__ import annotations

import json
import os
import shutil
from dataclasses import replace
from urllib.parse import urlparse

from dotenv import load_dotenv
from starlette.applications import Starlette
from starlette.requests import Request
from starlette.responses import JSONResponse, Response
from starlette.routing import Route

from app.config.settings import ConfigError, Settings, load_settings
from app.jobs import JobRunner
from app.ollama_proxy import proxy_base_url, start_proxy
from app.repos import RepoNotAllowed, RepoRegistry

DEFAULT_PORT = 8765
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

    # Preview-only unless the caller opts in. Matches the JobShout form default
    # so a missing field cannot spam GitHub.
    dry_run = True if "dry_run" not in payload else bool(payload.get("dry_run"))
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

    import uvicorn

    uvicorn.run(app, host="0.0.0.0", port=port, log_level="info")


if __name__ == "__main__":
    main()
