"""Langfuse tracing for LLM runs.

Tracing is on only when both Langfuse keys are configured; otherwise every
helper degrades to a no-op and the runners behave exactly as before. The
sidecar therefore never *requires* a Langfuse deployment — see
docs/langfuse.md for how to run one locally.
"""

from __future__ import annotations

import logging
from contextlib import nullcontext
from typing import Any

from app.config import settings
from app.models import RunRequest

logger = logging.getLogger(__name__)

_client: Any = None


def enabled() -> bool:
    """Whether Langfuse tracing is configured."""
    return bool(settings.langfuse_public_key and settings.langfuse_secret_key)


def _ensure_client() -> Any:
    """Create the singleton Langfuse client on first use."""
    global _client
    if _client is None:
        from langfuse import Langfuse

        _client = Langfuse(
            public_key=settings.langfuse_public_key,
            secret_key=settings.langfuse_secret_key,
            host=settings.langfuse_host or None,
        )
        logger.info("Langfuse tracing enabled (host=%s)", settings.langfuse_host)
    return _client


def run_context(req: RunRequest, trace_name: str):
    """Context manager stamping trace attributes on every span of a run.

    The JobShout execution id becomes the Langfuse session (so retries of one
    execution group together) and the agent id the Langfuse user (so the
    dashboard can slice by agent). propagate_attributes rather than invoke
    metadata because only it reaches *child* spans — a LangGraph run nests its
    generations under per-node chains, and metadata-only attribution leaves
    them unattributed.
    """
    if not enabled():
        return nullcontext()

    _ensure_client()
    from langfuse import propagate_attributes

    return propagate_attributes(
        user_id=req.agent_id,
        session_id=req.execution_id,
        trace_name=trace_name,
        tags=[t for t in (req.provider or "ollama", req.model) if t],
    )


def invoke_config(req: RunRequest, trace_name: str) -> dict:
    """LangChain RunnableConfig that traces a run under `trace_name`.

    Returns {} when tracing is off, which every LangChain/LangGraph invoke
    accepts as "no config". Use together with run_context().
    """
    if not enabled():
        return {}

    _ensure_client()
    from langfuse.langchain import CallbackHandler

    return {
        "callbacks": [CallbackHandler(public_key=settings.langfuse_public_key)],
        "run_name": trace_name,
    }


def flush() -> None:
    """Push queued events out now.

    Called at the end of each run: the SDK batches in the background, and at
    this sidecar's volume one blocking flush per execution is cheaper than
    losing the tail of a trace to a container stop.
    """
    if _client is not None:
        try:
            _client.flush()
        except Exception:  # never let telemetry break a run
            logger.exception("Langfuse flush failed")
