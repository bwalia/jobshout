"""LangChain execution runner — constructs and runs chains/agents via Ollama or OpenAI."""

from __future__ import annotations

import logging
import time
from typing import Any

from langchain_core.messages import HumanMessage, SystemMessage
from langchain_core.prompts import ChatPromptTemplate

from app import observability
from app.llm import get_llm
from app.models import RunRequest, RunResponse, ToolCallRecord

logger = logging.getLogger(__name__)


def run(req: RunRequest) -> RunResponse:
    """Execute a LangChain prompt chain and return the result."""
    start = time.monotonic()

    try:
        llm = get_llm(req)
        run_config = observability.invoke_config(req, "langchain-run")

        # Build a simple prompt chain.
        messages: list[Any] = []
        if req.system_prompt:
            messages.append(SystemMessage(content=req.system_prompt))
        messages.append(HumanMessage(content=req.prompt))

        # Check for template in config.
        template_str = req.config.get("template")
        with observability.run_context(req, "langchain-run"):
            if template_str:
                template_inputs = req.config.get("template_inputs", {})
                prompt = ChatPromptTemplate.from_template(template_str)
                chain = prompt | llm
                result = chain.invoke(template_inputs, config=run_config)
            else:
                result = llm.invoke(messages, config=run_config)

        # Extract content from the response.
        if hasattr(result, "content"):
            final_answer = result.content
        else:
            final_answer = str(result)

        # Approximate token count from response metadata.
        total_tokens = 0
        if hasattr(result, "response_metadata"):
            meta = result.response_metadata or {}
            usage = meta.get("token_usage") or meta.get("usage") or {}
            total_tokens = usage.get("total_tokens", 0)

        elapsed_ms = int((time.monotonic() - start) * 1000)

        return RunResponse(
            execution_id=req.execution_id,
            final_answer=final_answer,
            iterations=1,
            total_tokens=total_tokens,
            tool_calls=[
                ToolCallRecord(
                    tool_name="langchain_invoke",
                    input={"prompt": req.prompt[:200]},
                    output=final_answer[:500],
                    duration_ms=elapsed_ms,
                )
            ],
        )

    except Exception as exc:
        logger.exception("LangChain execution failed")
        return RunResponse(
            execution_id=req.execution_id,
            error=str(exc),
        )
    finally:
        observability.flush()
