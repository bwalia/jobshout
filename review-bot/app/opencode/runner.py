"""Run OpenCode headlessly and recover the structured review from its output."""

from __future__ import annotations

import json
import os
import re
import subprocess
from pathlib import Path
from urllib.error import HTTPError, URLError
from urllib.request import Request, urlopen

from app.config.settings import Settings
from app.opencode.provider import AGENT_ID, config_file, model_ref
from app.prompts.review import build_coercion_prompt

_ANSI_RE = re.compile(r"\x1b\[[0-9;?]*[a-zA-Z]")

DEFAULT_TIMEOUT = 900  # A 30B model exploring a repo is slow; don't cut it off early.
COERCE_TIMEOUT = 180  # The reformat pass is a single, tool-free completion — it is quick.


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


def coerce_review_json(settings: Settings, review_text: str, timeout: int = COERCE_TIMEOUT) -> dict:
    """Reformat a prose review into the review schema via one tool-free completion.

    OpenCode is not involved here: this is a direct OpenAI-compatible call to the
    same Ollama endpoint the agent uses (settings.ollama_host is the in-pod proxy at
    runtime, so gateway auth is handled). temperature=0 keeps the reformat faithful.
    """
    url = f"{settings.ollama_host}/v1/chat/completions"
    body = json.dumps(
        {
            "model": settings.model,
            "messages": [{"role": "user", "content": build_coercion_prompt(review_text)}],
            "stream": False,
            "temperature": 0,
        }
    ).encode()
    request = Request(url, data=body, method="POST")
    request.add_header("Content-Type", "application/json")
    try:
        with urlopen(request, timeout=timeout) as response:
            data = json.loads(response.read())
    except (HTTPError, URLError, TimeoutError, json.JSONDecodeError) as exc:
        raise OpenCodeError(f"JSON reformat call to Ollama failed: {exc}") from exc

    choices = data.get("choices") or [{}]
    content = choices[0].get("message", {}).get("content", "") or ""
    return extract_review_json(content)


def run_review(settings: Settings, workdir: Path, prompt: str, timeout: int = DEFAULT_TIMEOUT) -> dict:
    """Run OpenCode in workdir; it explores the repo and returns the review JSON.

    qwen3-coder follows the long review prompt's "explore and explain" framing and
    reliably answers in prose, dropping the trailing "output JSON" rule — a
    deterministic failure on clean PRs, so retrying the same prompt just fails again.
    Instead we take the prose it produced and reformat it into the schema with a
    short, single-purpose call the model does obey. Only if both the first pass and
    the reformat yield nothing usable do we re-run the agent once.
    """
    raw = run_agent(settings, workdir, prompt, timeout)
    try:
        return extract_review_json(raw)
    except NoReviewJSON:
        pass

    prose = _ANSI_RE.sub("", raw).strip()
    if prose:
        try:
            return coerce_review_json(settings, prose)
        except NoReviewJSON:
            pass

    # First pass was empty or garbled and the reformat had nothing to work with.
    # A fresh run is the only remaining option.
    return extract_review_json(run_agent(settings, workdir, prompt, timeout))
