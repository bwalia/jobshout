"""JobShout Pentest Service — Strix behind HTTP.

See README.md for why this runs on the workstation rather than in the cluster,
and scope.py for what stops it becoming an open scanning relay.

The image service blocks for the ~25 seconds an image takes and returns the
picture. A scan takes minutes to hours, which no HTTP path through Cloudflare
and pop0 will hold open, so this one is start-and-poll: POST returns a run id
immediately and the caller polls GET until the status is terminal.
"""

from __future__ import annotations

import asyncio
import contextlib
import logging
import os
import shutil
import subprocess
from contextlib import asynccontextmanager
from pathlib import Path

from fastapi import Depends, FastAPI, HTTPException, Response, status
from pydantic import BaseModel, Field

from app import config, scope
from app.auth import require_auth
from app.runner import Busy, runner
from app.store import store

logging.basicConfig(
    level=getattr(logging, config.LOG_LEVEL.upper(), logging.INFO),
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)

SCAN_MODES = ("quick", "standard", "deep")


@asynccontextmanager
async def lifespan(app: FastAPI):
    store.load_from_disk()
    store.reap()
    if not scope.RULES:
        logger.warning(
            "STRIX_TARGET_ALLOWLIST is empty — every scan request will be refused. "
            "This is the safe default, not a malfunction."
        )
    elif any(r.kind == "any" for r in scope.RULES):
        logger.warning(
            "STRIX_TARGET_ALLOWLIST includes * — any public host may be scanned. "
            "Private / loopback / metadata addresses are still refused unless named."
        )
    logger.info(
        "pentest service ready: llm=%s api_base=%s concurrency=%d allowlist=%d rule(s) auth=%s",
        config.LLM, config.LLM_API_BASE, config.MAX_CONCURRENT,
        len(scope.RULES), "on" if config.JWT_SECRET else "OFF",
    )
    _assert_toolcall_patch()
    # Warm the model in the background so startup is not blocked on a 30–60 s cold
    # load while a first scan is already being submitted.
    warm_task = asyncio.create_task(_warm_model())
    yield
    if not warm_task.done():
        warm_task.cancel()
    with contextlib.suppress(asyncio.CancelledError):
        await warm_task
    await runner.drain()


def _assert_toolcall_patch() -> None:
    """Log whether the LiteLLM tool-call patch is wired onto the scan subprocess.

    The patch (patches/sitecustomize.py) runs inside the *Strix* process, not
    this one — runner.build_env puts its directory on PYTHONPATH so Python
    auto-imports it there. We cannot import litellm here to confirm the live
    monkeypatch (litellm lives in Strix's environment, not the service's), so we
    assert the load path instead: the file exists, still defines the tolerant
    loader, and its directory is on the env every scan inherits. A missing or
    unwired patch is the failure that silently kills every qwen3-coder scan
    mid-run, so it is a warning, not a debug line.
    """
    patch_dir = Path(__file__).resolve().parent.parent / "patches"
    patch_file = patch_dir / "sitecustomize.py"
    env_pythonpath = runner.build_env().get("PYTHONPATH", "")
    on_path = str(patch_dir) in env_pythonpath.split(os.pathsep)
    defines_loader = patch_file.exists() and "_tolerant_loads" in patch_file.read_text()
    if defines_loader and on_path:
        logger.info("tool-call patch wired: %s on scan PYTHONPATH", patch_file)
    else:
        logger.warning(
            "tool-call patch NOT wired (file_ok=%s on_pythonpath=%s) — qwen3-coder "
            "tool calls may crash the scanner mid-run; see patches/sitecustomize.py",
            defines_loader, on_path,
        )


async def _warm_model() -> None:
    """Load the model into Ollama and hold it, so the first scan starts promptly.

    Best-effort and off the request path: a failure here only means the first
    scan pays the cold-load it would have paid anyway. A no-op for a hosted
    provider, which has no local endpoint to warm.
    """
    if not (config.WARM_MODEL_ON_START and config.LLM_API_BASE):
        return
    model = config.LLM.split("/", 1)[-1]

    def _post() -> None:
        import json as _json
        import urllib.request

        body = _json.dumps({"model": model, "keep_alive": config.MODEL_KEEP_ALIVE}).encode()
        req = urllib.request.Request(
            f"{config.LLM_API_BASE.rstrip('/')}/api/generate",
            data=body,
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        # An empty-prompt generate loads (or keeps) the model and returns quickly;
        # the timeout covers a genuine cold load of a 30B model.
        with urllib.request.urlopen(req, timeout=180) as resp:
            resp.read()

    try:
        await asyncio.to_thread(_post)
        logger.info("model %s warmed and held (keep_alive=%s)", model, config.MODEL_KEEP_ALIVE)
    except Exception as exc:  # noqa: BLE001 — warming is best-effort, never fatal
        logger.warning("model warm-up failed; first scan pays the cold-load cost: %s", exc)


app = FastAPI(
    title="JobShout Pentest Service",
    description="Autonomous penetration testing for the JobShout platform, backed by Strix.",
    version="1.0.0",
    lifespan=lifespan,
)


# ─── models ─────────────────────────────────────────────────────────────────

class ScanRequest(BaseModel):
    target: str = Field(min_length=1, max_length=2000)
    scan_mode: str = "quick"
    max_budget: int | None = Field(default=None, ge=1, le=10000)
    # The caller's own run id. Doubles as the idempotency key — see below.
    run_ref: str = Field(default="", max_length=128)
    # Operator scope note. Combined with a standing engagement prompt and
    # passed to Strix as --instruction (confirmed via strix --help).
    instruction: str = Field(default="", max_length=4000)
    requested_by: str = Field(default="", max_length=128)


class ScanAccepted(BaseModel):
    run_id: str
    run_ref: str
    status: str
    queue_position: int = 0
    existing: bool = False


class FindingOut(BaseModel):
    id: str
    title: str
    description: str
    severity: str
    cvss_score: float
    category: str
    reproduction_steps: str


class ScanStatus(BaseModel):
    run_id: str
    run_ref: str
    status: str
    target: str
    scan_mode: str
    created_at: str
    started_at: str | None = None
    completed_at: str | None = None
    duration_ms: int = 0
    exit_code: int | None = None
    finding_count: int = 0
    findings: list[FindingOut] = []
    error: str | None = None
    log_tail: str = ""
    report_markdown: str = ""
    target_engaged: bool | None = None
    # Audit fields kept out of the response model by omission; to_dict includes
    # them but Pydantic ignores extras when building ScanStatus unless configured.


# ─── health ─────────────────────────────────────────────────────────────────

@app.get("/health")
async def health():
    """Liveness only.

    Deliberately unauthenticated, and deliberately does not probe Docker, Ollama
    or Strix. A health check that can fail for four different reasons cannot be
    used to diagnose any of them; the things it declines to check live on
    /api/capabilities, which is asked on purpose.
    """
    return {"status": "ok", "service": "jobshout-pentest-service"}


@app.get("/")
async def root():
    return {
        "service": "jobshout-pentest-service",
        "backend": "strix",
        "llm": config.LLM,
        "authenticated": bool(config.JWT_SECRET),
        "targets_in_scope": len(scope.RULES),
    }


@app.get("/api/capabilities")
async def capabilities(caller: str = Depends(require_auth)):
    """Everything a scan needs, checked deliberately rather than on every ping."""
    checks = await asyncio.gather(
        asyncio.to_thread(_probe_strix),
        asyncio.to_thread(_probe_docker),
        asyncio.to_thread(_probe_ollama),
    )
    strix_info, docker_info, llm_info = checks
    return {
        "strix": strix_info,
        "docker": docker_info,
        "llm": {"model": config.LLM, "api_base": config.LLM_API_BASE, **llm_info},
        "scope": scope.describe(),
        "capacity": {
            "max_concurrent": runner.max_concurrent,
            "queue_max": runner.queue_max,
            "queue_depth": runner.queue_depth,
            "max_runtime_seconds": runner.max_runtime,
            "max_runtime_by_mode": config.RUNTIME_BY_MODE,
        },
    }


def _probe_strix() -> dict:
    path = shutil.which(config.BIN) or config.BIN
    try:
        out = subprocess.run([config.BIN, "--version"], capture_output=True,
                             text=True, timeout=15)
        return {"available": out.returncode == 0, "path": path,
                "version": (out.stdout or out.stderr).strip()[:200]}
    except (OSError, subprocess.SubprocessError) as exc:
        return {"available": False, "path": path, "error": str(exc)}


def _probe_docker() -> dict:
    """Strix sandboxes each scan in containers, so no Docker means no scanning."""
    try:
        out = subprocess.run(["docker", "info", "--format", "{{.ServerVersion}}"],
                             capture_output=True, text=True, timeout=15)
        if out.returncode != 0:
            return {"available": False, "error": (out.stderr or "").strip()[:200]}
        return {"available": True, "server_version": out.stdout.strip()}
    except (OSError, subprocess.SubprocessError) as exc:
        return {"available": False, "error": str(exc)}


def _probe_ollama() -> dict:
    if not config.LLM_API_BASE:
        return {"reachable": None, "note": "hosted provider; no local endpoint configured"}
    import urllib.error
    import urllib.request
    try:
        with urllib.request.urlopen(f"{config.LLM_API_BASE.rstrip('/')}/api/tags", timeout=10) as resp:
            import json as _json
            models = [m.get("name") for m in _json.load(resp).get("models", [])]
        wanted = config.LLM.split("/", 1)[-1]
        return {"reachable": True, "model_present": wanted in models, "model_count": len(models)}
    except (urllib.error.URLError, OSError, ValueError) as exc:
        return {"reachable": False, "error": str(exc)}


# ─── scans ──────────────────────────────────────────────────────────────────

@app.post("/api/scan", response_model=ScanAccepted, status_code=status.HTTP_202_ACCEPTED)
async def start_scan(req: ScanRequest, response: Response, caller: str = Depends(require_auth)):
    scan_mode = req.scan_mode.strip().lower() or "quick"
    if scan_mode not in SCAN_MODES:
        raise HTTPException(
            status.HTTP_400_BAD_REQUEST,
            f"scan_mode must be one of {', '.join(SCAN_MODES)}",
        )

    # Idempotency before anything expensive. A reconciler that retries after a
    # dropped response must not buy a second hours-long scan of a live target.
    if req.run_ref:
        existing = store.get_by_ref(req.run_ref)
        if existing is not None:
            logger.info("scan for run_ref=%s already exists as %s", req.run_ref, existing.run_id)
            response.status_code = status.HTTP_200_OK
            return ScanAccepted(run_id=existing.run_id, run_ref=existing.run_ref,
                                status=existing.status, existing=True)

    # Scope, on the machine that would do the scanning. 403 and never retried.
    try:
        target = await asyncio.to_thread(scope.enforce, req.target)
    except scope.OutOfScope as exc:
        logger.warning("refused target %r from caller=%s: %s", req.target, caller, exc)
        raise HTTPException(status.HTTP_403_FORBIDDEN, str(exc))

    run = store.create(
        run_ref=req.run_ref,
        target=target.normalized,
        scan_mode=scan_mode,
        max_budget=req.max_budget,
        instruction=req.instruction,
        requested_by=req.requested_by,
        addresses=list(target.addresses),
    )

    try:
        position = runner.submit(run)
    except Busy as exc:
        store.finish(run, "failed", error=str(exc))
        raise HTTPException(
            status.HTTP_503_SERVICE_UNAVAILABLE, str(exc),
            headers={"Retry-After": "60"},
        )

    logger.info(
        "scan %s queued: caller=%s target=%s mode=%s ref=%s by=%s",
        run.run_id, caller, run.target, scan_mode, req.run_ref or "-",
        req.requested_by or "-",
    )
    return ScanAccepted(run_id=run.run_id, run_ref=run.run_ref,
                        status=run.status, queue_position=position)


@app.get("/api/scan/{run_id}", response_model=ScanStatus)
async def get_scan(run_id: str, caller: str = Depends(require_auth)):
    run = store.get(run_id)
    if run is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, f"no scan with id {run_id}")
    return ScanStatus(**run.to_dict())


@app.get("/api/scans")
async def list_scans(limit: int = 50, caller: str = Depends(require_auth)):
    runs = store.list(limit=max(1, min(limit, 500)))
    # Summaries only: a list endpoint that returned every finding of every run
    # would be the largest response this service ever produces, for the request
    # least likely to want it.
    return {
        "runs": [
            {
                "run_id": r.run_id, "run_ref": r.run_ref, "status": r.status,
                "target": r.target, "scan_mode": r.scan_mode,
                "created_at": r.created_at, "completed_at": r.completed_at,
                "finding_count": r.finding_count,
            }
            for r in runs
        ],
        "count": len(runs),
    }


@app.delete("/api/scan/{run_id}")
async def cancel_scan(run_id: str, caller: str = Depends(require_auth)):
    run = store.get(run_id)
    if run is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, f"no scan with id {run_id}")
    cancelled = await runner.cancel(run)
    if not cancelled:
        return {"run_id": run_id, "status": run.status, "cancelled": False,
                "detail": "run had already finished"}
    logger.info("scan %s cancelled by caller=%s", run_id, caller)
    return {"run_id": run_id, "status": run.status, "cancelled": True}
