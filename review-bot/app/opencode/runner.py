"""Run OpenCode headlessly and recover the structured review from its output."""

from __future__ import annotations

import json
import os
import re
import subprocess
from pathlib import Path

from app.config.settings import Settings
from app.opencode.provider import AGENT_ID, config_file, model_ref

_ANSI_RE = re.compile(r"\x1b\[[0-9;?]*[a-zA-Z]")

DEFAULT_TIMEOUT = 900  # A 30B model exploring a repo is slow; don't cut it off early.


class OpenCodeError(RuntimeError):
    """Raised when OpenCode fails or returns no usable review."""


class NoReviewJSON(OpenCodeError):
    """The run finished but produced no parseable review JSON (often a malformed
    tool call emitted as text). Distinct because it is worth retrying, unlike a
    timeout or a non-zero exit."""


def _iter_json_objects(text: str):
    """Yield every balanced {...} block in text, respecting strings and escapes."""
    for start in (i for i, ch in enumerate(text) if ch == "{"):
        depth = 0
        in_string = False
        escaped = False
        for index in range(start, len(text)):
            char = text[index]
            if in_string:
                if escaped:
                    escaped = False
                elif char == "\\":
                    escaped = True
                elif char == '"':
                    in_string = False
                continue
            if char == '"':
                in_string = True
            elif char == "{":
                depth += 1
            elif char == "}":
                depth -= 1
                if depth == 0:
                    yield text[start : index + 1]
                    break


def extract_review_json(output: str) -> dict:
    """Pull the review object out of OpenCode's output.

    The output also carries tool-call lines and status headers, so we take the
    last balanced object that actually looks like a review.
    """
    clean = _ANSI_RE.sub("", output)
    found = None

    for candidate in _iter_json_objects(clean):
        try:
            parsed = json.loads(candidate)
        except json.JSONDecodeError:
            continue
        if isinstance(parsed, dict) and ("issues" in parsed or "summary" in parsed):
            found = parsed  # keep scanning; the final answer is the last one

    if found is None:
        raise NoReviewJSON(f"No review JSON found in OpenCode output:\n{clean[-2000:]}")
    return found


def run_agent(settings: Settings, workdir: Path, prompt: str, timeout: int = DEFAULT_TIMEOUT) -> str:
    """Run the read-only reviewer agent in workdir and return its raw stdout.

    Shared by the review flow (which then parses JSON) and the repo-map build
    (which keeps the markdown as-is).
    """
    with config_file(settings) as cfg:
        env = {**os.environ, "OPENCODE_CONFIG": str(cfg)}
        command = [
            "opencode",
            "run",
            "--auto",  # auto-approve tool permissions (required headless)
            "--agent",
            AGENT_ID,  # read-only reviewer, not the default all-tools "build" agent
            "-m",
            model_ref(settings),
            "--dir",
            str(workdir),
            prompt,
        ]
        try:
            result = subprocess.run(
                command,
                cwd=workdir,
                env=env,
                capture_output=True,
                text=True,
                timeout=timeout,
            )
        except FileNotFoundError as exc:
            raise OpenCodeError("`opencode` not found on PATH.") from exc
        except subprocess.TimeoutExpired as exc:
            raise OpenCodeError(f"OpenCode timed out after {timeout}s.") from exc

    if result.returncode != 0:
        raise OpenCodeError(f"OpenCode exited {result.returncode}:\n{result.stderr[-2000:]}")

    return result.stdout


def run_review(settings: Settings, workdir: Path, prompt: str, timeout: int = DEFAULT_TIMEOUT) -> dict:
    """Run OpenCode in workdir; it explores the repo and returns the review JSON.

    qwen3-coder intermittently emits a tool call as literal text instead of invoking
    it, which yields no review JSON. That failure is stochastic, so we retry once
    before giving up rather than failing an otherwise-good review.
    """
    try:
        return extract_review_json(run_agent(settings, workdir, prompt, timeout))
    except NoReviewJSON:
        # Retry only this failure mode — not timeouts or process errors.
        return extract_review_json(run_agent(settings, workdir, prompt, timeout))
