"""Running Strix, one scan at a time.

Strix is a command-line program that spawns Docker containers and talks to an
LLM for minutes to hours. This module is what stands between an HTTP request and
that subprocess: it bounds how many run at once, bounds how long any one may
take, captures what it said, and turns its exit code into a status.

Nothing here decides *whether* a target may be scanned. That is scope.py's job
and it has already run by the time anything in this file executes.
"""

from __future__ import annotations

import asyncio
import contextlib
import json
import logging
import os
import signal
from pathlib import Path

from app import config, store as store_module
from app.store import BUDGET_EXCEEDED, CANCELLED, COMPLETED, FAILED, RUNNING, Finding, Run

logger = logging.getLogger(__name__)

# The severity vocabulary the caller's database accepts (migration 026).
# Anything outside it is recorded as "info" rather than being guessed upward:
# inventing a severity is worse than admitting the scanner did not give one.
SEVERITIES = ("critical", "high", "medium", "low", "info")
_SEVERITY_ALIASES = {
    "informational": "info",
    "information": "info",
    "note": "info",
    "none": "info",
    "warning": "medium",
    "moderate": "medium",
    "important": "high",
    "severe": "critical",
}


class Busy(Exception):
    """The queue is full. Transient — the request was fine, try later."""


def _normalize_severity(raw: object) -> str:
    value = str(raw or "").strip().lower()
    value = _SEVERITY_ALIASES.get(value, value)
    return value if value in SEVERITIES else "info"


def _coerce_float(raw: object) -> float:
    try:
        return float(raw)  # type: ignore[arg-type]
    except (TypeError, ValueError):
        return 0.0


def parse_findings(run_dir: Path) -> list[Finding]:
    """Read Strix's vulnerabilities.json out of a run directory.

    Strix is executed with the run's own directory as its working directory, so
    whatever it wrote beneath there belongs to this run and no other. That is
    the whole reason for the per-run cwd — the alternative, scraping a run id out
    of stdout and hoping the newest directory is the right one, is what the Go
    client used to do and it was never reliable.
    """
    candidates = sorted(
        run_dir.rglob("vulnerabilities.json"),
        key=lambda p: p.stat().st_mtime if p.exists() else 0,
        reverse=True,
    )
    if not candidates:
        return []

    try:
        data = json.loads(candidates[0].read_text())
    except (OSError, ValueError) as exc:
        logger.warning("could not read %s: %s", candidates[0], exc)
        return []

    # Accept both the documented {"vulnerabilities": [...]} envelope and a bare
    # list, because a scanner that changes its output shape should degrade to
    # "no findings parsed" rather than to a 500.
    entries = data.get("vulnerabilities", []) if isinstance(data, dict) else data
    if not isinstance(entries, list):
        return []

    findings = []
    for entry in entries:
        if not isinstance(entry, dict):
            continue
        findings.append(
            Finding(
                id=str(entry.get("id", "")),
                title=str(entry.get("title", "")).strip() or "(untitled finding)",
                description=str(entry.get("description", "")),
                severity=_normalize_severity(entry.get("severity")),
                cvss_score=_coerce_float(entry.get("cvss_score")),
                category=str(entry.get("category", "")),
                reproduction_steps=str(entry.get("reproduction_steps", "")),
            )
        )
    return findings


class Runner:
    """Bounded execution of scans."""

    def __init__(self, store=None, *, max_concurrent: int | None = None,
                 queue_max: int | None = None, max_runtime: int | None = None):
        self.store = store or store_module.store
        self.max_concurrent = max_concurrent or config.MAX_CONCURRENT
        self.queue_max = queue_max or config.QUEUE_MAX
        self.max_runtime = max_runtime or config.MAX_RUNTIME_SECONDS

        self._semaphore = asyncio.Semaphore(self.max_concurrent)
        self._waiting = 0
        self._tasks: set[asyncio.Task] = set()
        self._processes: dict[str, asyncio.subprocess.Process] = {}
        self._cancelled: set[str] = set()

    # ─── admission ──────────────────────────────────────────────────────────

    @property
    def queue_depth(self) -> int:
        return self._waiting

    def submit(self, run: Run) -> int:
        """Schedule a run, returning its position in the queue.

        Raises Busy when the queue is full. That is answered upstream with a 503
        and a Retry-After rather than a 500: the request was valid and trying
        again later is the correct response to it.
        """
        if self._waiting >= self.queue_max:
            raise Busy(
                f"{self._waiting} scans already queued (limit {self.queue_max})"
            )
        position = self._waiting
        self._waiting += 1

        task = asyncio.create_task(self._run(run))
        self._tasks.add(task)
        task.add_done_callback(self._tasks.discard)
        return position

    # ─── execution ──────────────────────────────────────────────────────────

    async def _run(self, run: Run) -> None:
        try:
            async with self._semaphore:
                self._waiting = max(0, self._waiting - 1)
                if run.run_id in self._cancelled:
                    self.store.finish(run, CANCELLED, error="cancelled before it started")
                    return
                await self._execute(run)
        except Exception as exc:  # noqa: BLE001 — the boundary; a task that dies silently strands a run forever
            logger.exception("run %s failed unexpectedly", run.run_id)
            self.store.finish(run, FAILED, error=f"pentest service error: {exc}")
        finally:
            self._processes.pop(run.run_id, None)
            self._cancelled.discard(run.run_id)

    def build_args(self, run: Run) -> list[str]:
        # Only flags the Go client already used against a real Strix. The
        # instruction is deliberately not passed: this service has no verified
        # flag for it, and an invented one would fail every scan rather than
        # just ignoring the field. It is kept on the run record for the audit
        # trail, and STRIX_EXTRA_ARGS exists for operators who know better.
        args = [config.BIN, "-n", "--target", run.target, "--scan-mode", run.scan_mode]
        if run.max_budget:
            args += ["--max-budget", str(run.max_budget)]
        args += config.EXTRA_ARGS
        return args

    def build_env(self) -> dict[str, str]:
        env = dict(os.environ)
        env["STRIX_LLM"] = config.LLM
        if config.LLM_API_KEY:
            env["LLM_API_KEY"] = config.LLM_API_KEY
        # Only when configured, so a hosted provider keeps its own default
        # endpoint instead of being pointed at an empty string.
        if config.LLM_API_BASE:
            env["LLM_API_BASE"] = config.LLM_API_BASE
        return env

    async def _execute(self, run: Run) -> None:
        run_dir = self.store.run_dir(run.run_id)
        run_dir.mkdir(parents=True, exist_ok=True)
        log_path = run_dir / "strix.log"

        args = self.build_args(run)
        self.store.update(run, status=RUNNING, started_at=store_module.now())
        logger.info("scan %s starting: %s", run.run_id, " ".join(args))

        try:
            proc = await asyncio.create_subprocess_exec(
                *args,
                cwd=run_dir,
                env=self.build_env(),
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.STDOUT,
                # Its own process group, so a timeout can take down Strix and
                # everything it spawned rather than orphaning the children.
                start_new_session=True,
            )
        except FileNotFoundError:
            self.store.finish(
                run, FAILED,
                error=(
                    f"strix binary {config.BIN!r} not found — install Strix and set "
                    f"STRIX_BIN, and note that a launchd agent inherits a minimal PATH"
                ),
            )
            return
        except OSError as exc:
            self.store.finish(run, FAILED, error=f"could not start strix: {exc}")
            return

        self._processes[run.run_id] = proc
        pump = asyncio.create_task(self._pump(proc, log_path))

        timed_out = False
        try:
            await asyncio.wait_for(proc.wait(), timeout=self.max_runtime)
        except asyncio.TimeoutError:
            timed_out = True
            logger.warning("scan %s exceeded %ss — terminating", run.run_id, self.max_runtime)
            await self._terminate(proc)

        tail = await pump
        exit_code = proc.returncode

        if run.run_id in self._cancelled:
            self.store.finish(run, CANCELLED, exit_code=exit_code, log_tail=tail,
                              error="cancelled")
            return

        if timed_out:
            self.store.finish(
                run, FAILED, exit_code=exit_code, log_tail=tail,
                error=(
                    f"scan exceeded the {self.max_runtime}s runtime limit and was "
                    f"terminated; any containers Strix left behind may need "
                    f"clearing with `docker ps`"
                ),
            )
            return

        findings = parse_findings(run_dir)
        status, error = self._classify(exit_code, tail, findings)
        self.store.finish(
            run, status,
            exit_code=exit_code,
            findings=findings,
            finding_count=len(findings),
            log_tail=tail,
            error=error,
        )
        logger.info(
            "scan %s finished: status=%s exit=%s findings=%d",
            run.run_id, status, exit_code, len(findings),
        )

    def _classify(self, exit_code: int | None, output: str,
                  findings: list[Finding]) -> tuple[str, str | None]:
        """Turn an exit code into a status.

        Strix documents 0 as a clean scan and 2 as "vulnerabilities found" —
        both are successful scans, and conflating 2 with failure would report
        every scan that actually found something as broken.
        """
        if exit_code in (0, 2):
            return COMPLETED, None

        lowered = output.lower()
        # Both words, not either: the Go client tested for "budget" OR
        # "exceeded", which matched any run whose output mentioned a rate limit
        # being exceeded and mislabelled it as a budget stop.
        if "budget" in lowered and "exceeded" in lowered:
            return BUDGET_EXCEEDED, "scan stopped at its budget cap"

        snippet = output.strip()[-400:]
        return FAILED, f"strix exited with code {exit_code}: {snippet}" if snippet else \
            f"strix exited with code {exit_code} and produced no output"

    async def _pump(self, proc: asyncio.subprocess.Process, log_path: Path) -> str:
        """Stream the process output to disk, keeping the tail in memory.

        The full log goes to a file because when a scan fails at 3am the output
        is the difference between a diagnosis and a shrug. Only the tail is kept
        in memory, because the run registry is not a log store.
        """
        tail = bytearray()
        try:
            with open(log_path, "ab") as handle:
                while proc.stdout is not None:
                    chunk = await proc.stdout.read(4096)
                    if not chunk:
                        break
                    handle.write(chunk)
                    handle.flush()
                    tail.extend(chunk)
                    if len(tail) > config.LOG_TAIL_BYTES:
                        del tail[: -config.LOG_TAIL_BYTES]
        except OSError as exc:
            logger.warning("could not write %s: %s", log_path, exc)
        return tail.decode("utf-8", errors="replace")

    async def _terminate(self, proc: asyncio.subprocess.Process) -> None:
        """SIGTERM the process group, then SIGKILL what is left of it."""
        if proc.returncode is not None:
            return
        for sig in (signal.SIGTERM, signal.SIGKILL):
            try:
                os.killpg(os.getpgid(proc.pid), sig)
            except (ProcessLookupError, PermissionError):
                return
            with contextlib.suppress(asyncio.TimeoutError):
                await asyncio.wait_for(proc.wait(), timeout=config.TERM_GRACE_SECONDS)
                return

    # ─── cancellation ───────────────────────────────────────────────────────

    async def cancel(self, run: Run) -> bool:
        """Ask a run to stop. False when it had already finished."""
        if run.is_terminal:
            return False
        self._cancelled.add(run.run_id)
        proc = self._processes.get(run.run_id)
        if proc is not None:
            await self._terminate(proc)
        return True

    async def drain(self) -> None:
        """Stop everything in flight, for shutdown."""
        for run_id, proc in list(self._processes.items()):
            self._cancelled.add(run_id)
            await self._terminate(proc)
        if self._tasks:
            await asyncio.gather(*list(self._tasks), return_exceptions=True)


runner = Runner()
