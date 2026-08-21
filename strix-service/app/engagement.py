"""Detect whether a Strix scan actually engaged the target host.

A hollow run exits cleanly with zero findings after only listing the sandbox —
the failure mode we saw on int.jobshout.co.uk. Prefer fail-closed: completed
with no engagement is not a "Clean" security result.
"""

from __future__ import annotations

import json
import logging
import re
import sqlite3
from pathlib import Path
from urllib.parse import urlsplit

logger = logging.getLogger(__name__)

# Phrases that look like real HTTP work — not merely the --target URL in argv.
_HTTP_HINTS = re.compile(
    r"(curl\b|wget\b|fetch\b|GET\s+/|POST\s+/|HEAD\s+/|"
    r"requests\.(get|post|head|put|delete)|httpx\.|urllib|"
    r"HTTP/[12]|status[_\s]?code|response\.status|"
    r"Connecting to |Connected to )",
    re.IGNORECASE,
)

STANDING_ENGAGEMENT = (
    "You are testing ONLY the target provided via --target. "
    "Start with HTTP reconnaissance against that origin: fetch the homepage, "
    "inspect response headers, try robots.txt and a few well-known paths. "
    "Do not invent other brands, Acme portals, or example.com narratives. "
    "Do not call finish_scan until you have interacted with the target over "
    "HTTP (or recorded an explicit network/DNS blocker). "
    "Report only confirmed findings against this target."
)


def host_of(target: str) -> str:
    raw = (target or "").strip()
    if not raw:
        return ""
    parseable = raw if "://" in raw else f"//{raw}"
    try:
        host = urlsplit(parseable).hostname
    except ValueError:
        return ""
    if not host:
        return ""
    return host.lower().strip(".")


def compose_instruction(target: str, operator_note: str = "") -> str:
    """Standing engagement prompt, plus any operator scope note."""
    parts = [STANDING_ENGAGEMENT, f"Target: {target.strip()}"]
    note = (operator_note or "").strip()
    if note:
        parts.append(f"Operator scope note: {note}")
    return "\n\n".join(parts)


def read_report_markdown(run_dir: Path) -> str:
    """Newest penetration_test_report.md under the run directory, if any."""
    candidates = sorted(
        run_dir.rglob("penetration_test_report.md"),
        key=lambda p: p.stat().st_mtime if p.exists() else 0,
        reverse=True,
    )
    if not candidates:
        return ""
    try:
        text = candidates[0].read_text(encoding="utf-8", errors="replace")
    except OSError as exc:
        logger.warning("could not read report %s: %s", candidates[0], exc)
        return ""
    # Cap so a runaway report cannot blow the API response.
    if len(text) > 200_000:
        return text[:200_000] + "\n\n…[truncated]"
    return text


def target_engaged(run_dir: Path, target: str, log_tail: str = "") -> bool:
    """True when artifacts show the scanner reached the target host."""
    host = host_of(target)
    if not host:
        return False

    blob_parts: list[str] = []
    if log_tail:
        blob_parts.append(log_tail)

    log_path = run_dir / "strix.log"
    if log_path.is_file():
        try:
            # First 256KiB is enough to see early recon; full logs can be huge.
            blob_parts.append(log_path.read_text(encoding="utf-8", errors="replace")[:262_144])
        except OSError:
            pass

    for db_path in run_dir.rglob("agents.db"):
        blob_parts.append(_sqlite_text(db_path))

    for json_path in run_dir.rglob("run.json"):
        try:
            blob_parts.append(json_path.read_text(encoding="utf-8", errors="replace")[:131_072])
        except OSError:
            pass

    blob = "\n".join(blob_parts)
    if host not in blob.lower():
        return False
    # Host alone is not enough (it appears in the CLI args). Require an HTTP-ish hint nearby.
    return bool(_HTTP_HINTS.search(blob))


def _sqlite_text(path: Path) -> str:
    try:
        con = sqlite3.connect(f"file:{path}?mode=ro", uri=True)
    except sqlite3.Error:
        return ""
    try:
        rows = con.execute(
            "SELECT name FROM sqlite_master WHERE type='table'"
        ).fetchall()
        chunks: list[str] = []
        for (table,) in rows:
            if table.startswith("sqlite_"):
                continue
            try:
                for row in con.execute(f"SELECT * FROM {table} LIMIT 500"):
                    chunks.append(" ".join(str(c) for c in row if c is not None))
            except sqlite3.Error:
                continue
        return "\n".join(chunks)
    finally:
        con.close()
