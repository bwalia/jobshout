"""The run registry.

Runs live in memory and are mirrored to ``<runs_dir>/<run_id>/meta.json`` after
every state change. The mirror is not a database — Postgres on the caller's side
is the system of record — it exists so that a service restart can still answer
"what happened to run X" for work that completed before the restart, and can
tell the truth about work that did not.
"""

from __future__ import annotations

import json
import logging
import shutil
import threading
import time
import uuid
from dataclasses import asdict, dataclass, field
from datetime import datetime, timedelta, timezone
from pathlib import Path

from app import config

logger = logging.getLogger(__name__)

# Statuses a run can hold. These are the strings the Go client maps onto
# pentest_runs.status, so they match migration 026's CHECK constraint.
QUEUED = "queued"
RUNNING = "running"
COMPLETED = "completed"
FAILED = "failed"
BUDGET_EXCEEDED = "budget_exceeded"
CANCELLED = "cancelled"

TERMINAL = frozenset({COMPLETED, FAILED, BUDGET_EXCEEDED, CANCELLED})


def now() -> str:
    return datetime.now(timezone.utc).isoformat()


@dataclass
class Finding:
    # Field names match strix.Finding on the Go side exactly, so a result needs
    # no translation layer in between.
    id: str = ""
    title: str = ""
    description: str = ""
    severity: str = ""
    cvss_score: float = 0.0
    category: str = ""
    reproduction_steps: str = ""


@dataclass
class Run:
    run_id: str
    run_ref: str
    target: str
    scan_mode: str
    max_budget: int | None = None
    # Operator scope note. Combined with a standing engagement prompt and passed
    # to Strix as --instruction (see engagement.py / runner.build_args).
    instruction: str = ""
    requested_by: str = ""
    addresses: list[str] = field(default_factory=list)

    status: str = QUEUED
    created_at: str = field(default_factory=now)
    started_at: str | None = None
    completed_at: str | None = None
    duration_ms: int = 0
    exit_code: int | None = None
    finding_count: int = 0
    findings: list[Finding] = field(default_factory=list)
    error: str | None = None
    log_tail: str = ""
    report_markdown: str = ""
    target_engaged: bool | None = None

    @property
    def is_terminal(self) -> bool:
        return self.status in TERMINAL

    def to_dict(self) -> dict:
        return asdict(self)


class Store:
    """Thread-safe registry of runs, mirrored to disk."""

    def __init__(self, runs_dir: Path | None = None):
        self._runs: dict[str, Run] = {}
        self._by_ref: dict[str, str] = {}
        self._lock = threading.RLock()
        self.runs_dir = runs_dir or config.RUNS_DIR
        self.runs_dir.mkdir(parents=True, exist_ok=True)

    # ─── lifecycle ──────────────────────────────────────────────────────────

    def create(self, *, run_ref: str, target: str, scan_mode: str,
               max_budget: int | None, instruction: str, requested_by: str,
               addresses: list[str]) -> Run:
        with self._lock:
            run = Run(
                run_id=uuid.uuid4().hex,
                run_ref=run_ref,
                target=target,
                scan_mode=scan_mode,
                max_budget=max_budget,
                instruction=instruction,
                requested_by=requested_by,
                addresses=addresses,
            )
            self._runs[run.run_id] = run
            if run_ref:
                self._by_ref[run_ref] = run.run_id
            self.run_dir(run.run_id).mkdir(parents=True, exist_ok=True)
            self._persist(run)
            return run

    def get(self, run_id: str) -> Run | None:
        with self._lock:
            return self._runs.get(run_id)

    def get_by_ref(self, run_ref: str) -> Run | None:
        """Look up by the caller's own run id — the idempotency key.

        Without this, one dropped response during a reconciler retry costs a
        duplicate scan against a live target: the expensive kind of mistake to
        make twice.
        """
        with self._lock:
            run_id = self._by_ref.get(run_ref)
            return self._runs.get(run_id) if run_id else None

    def list(self, limit: int = 50) -> list[Run]:
        with self._lock:
            runs = sorted(self._runs.values(), key=lambda r: r.created_at, reverse=True)
            return runs[:limit]

    def update(self, run: Run, **changes) -> Run:
        with self._lock:
            for key, value in changes.items():
                setattr(run, key, value)
            self._persist(run)
            return run

    def finish(self, run: Run, status: str, **changes) -> Run:
        """Move a run to a terminal state, stamping the timing."""
        with self._lock:
            changes["status"] = status
            changes["completed_at"] = now()
            if run.started_at:
                started = datetime.fromisoformat(run.started_at)
                completed = datetime.fromisoformat(changes["completed_at"])
                changes["duration_ms"] = int((completed - started).total_seconds() * 1000)
            return self.update(run, **changes)

    # ─── disk ───────────────────────────────────────────────────────────────

    def run_dir(self, run_id: str) -> Path:
        return self.runs_dir / run_id

    def _persist(self, run: Run) -> None:
        path = self.run_dir(run.run_id) / "meta.json"
        try:
            path.parent.mkdir(parents=True, exist_ok=True)
            # Written to a temp file and moved, so a crash mid-write leaves the
            # previous state rather than a truncated file that will not parse.
            tmp = path.with_suffix(".json.tmp")
            tmp.write_text(json.dumps(run.to_dict(), indent=2))
            tmp.replace(path)
        except OSError as exc:
            logger.warning("could not persist run %s: %s", run.run_id, exc)

    def load_from_disk(self) -> int:
        """Re-read runs left on disk by a previous process.

        Anything that was still queued or running is marked failed: its
        subprocess died with the process that spawned it, so reporting it as
        still running would be a lie that a poller would believe forever.
        """
        recovered = 0
        for meta_path in sorted(self.runs_dir.glob("*/meta.json")):
            try:
                data = json.loads(meta_path.read_text())
                findings = [Finding(**f) for f in data.pop("findings", [])]
                run = Run(**data)
                run.findings = findings
            except (OSError, ValueError, TypeError) as exc:
                logger.warning("skipping unreadable run at %s: %s", meta_path, exc)
                continue

            if not run.is_terminal:
                run.status = FAILED
                run.error = "the pentest service restarted while this scan was in flight"
                run.completed_at = now()

            with self._lock:
                self._runs[run.run_id] = run
                if run.run_ref:
                    self._by_ref[run.run_ref] = run.run_id
                if not run.is_terminal:
                    self._persist(run)
            recovered += 1

        if recovered:
            logger.info("recovered %d run(s) from %s", recovered, self.runs_dir)
        return recovered

    def reap(self, retention_days: int | None = None) -> int:
        """Delete artifacts older than the retention window.

        Findings are already copied into Postgres by the caller, and Strix's
        artifacts can contain credentials it discovered — so this is hygiene,
        not data loss.
        """
        days = config.RETENTION_DAYS if retention_days is None else retention_days
        if days <= 0:
            return 0

        cutoff = datetime.now(timezone.utc) - timedelta(days=days)
        removed = 0
        for run in self.list(limit=10_000):
            try:
                created = datetime.fromisoformat(run.created_at)
            except ValueError:
                continue
            if created >= cutoff or not run.is_terminal:
                continue
            try:
                shutil.rmtree(self.run_dir(run.run_id), ignore_errors=True)
            except OSError as exc:
                logger.warning("could not reap run %s: %s", run.run_id, exc)
                continue
            with self._lock:
                self._runs.pop(run.run_id, None)
                if run.run_ref:
                    self._by_ref.pop(run.run_ref, None)
            removed += 1

        if removed:
            logger.info("reaped %d run(s) older than %d days", removed, days)
        return removed


store = Store()
