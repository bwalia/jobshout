"""Generate the OpenCode config: the Ollama provider plus the reviewer agent.

OpenCode ships no Ollama provider by default, so we register one against Ollama's
OpenAI-compatible endpoint. This is written to a temp file and passed via
OPENCODE_CONFIG so the user's own OpenCode config is never modified.

We also define a dedicated read-only "reviewer" agent rather than using OpenCode's
default "build" agent, which grants every tool. See AGENT_TOOLS for why.
"""

from __future__ import annotations

import json
import tempfile
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator

from app.config.settings import Settings

PROVIDER_ID = "ollama"
AGENT_ID = "reviewer"

# Reviewing is read-only exploration. The default "build" agent enables every tool,
# which is wrong here for two reasons:
#   task       - spawns nested sub-agents; each is a full agent loop against the same
#                local model, which turns a minutes-long review into an unbounded one.
#   write/edit - a reviewer must never modify the code it is judging.
# todowrite/todoread are planning scaffolding that a single-shot review does not need.
AGENT_TOOLS = {
    "task": False,
    "todowrite": False,
    "todoread": False,
    "write": False,
    "edit": False,
    "patch": False,
    "read": True,
    "grep": True,
    "glob": True,
    "list": True,
    "bash": True,
}


def build_config(settings: Settings) -> dict:
    return {
        "$schema": "https://opencode.ai/config.json",
        "provider": {
            PROVIDER_ID: {
                "npm": "@ai-sdk/openai-compatible",
                "name": "Ollama (local)",
                "options": {"baseURL": f"{settings.ollama_host}/v1"},
                "models": {settings.model: {"name": settings.model, "tools": True}},
            }
        },
        "agent": {
            AGENT_ID: {
                "mode": "primary",
                "description": "Read-only pull request reviewer",
                "tools": AGENT_TOOLS,
            }
        },
    }


def model_ref(settings: Settings) -> str:
    """The provider/model string OpenCode expects."""
    return f"{PROVIDER_ID}/{settings.model}"


@contextmanager
def config_file(settings: Settings) -> Iterator[Path]:
    with tempfile.NamedTemporaryFile("w", suffix=".json", prefix="review-bot-oc-", delete=False) as handle:
        json.dump(build_config(settings), handle)
        path = Path(handle.name)
    try:
        yield path
    finally:
        path.unlink(missing_ok=True)
