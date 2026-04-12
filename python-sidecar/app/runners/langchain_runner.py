"""LangChain execution runner — constructs and runs chains/agents via Ollama or OpenAI."""

from __future__ import annotations

import logging
import time
from typing import Any

from langchain_core.messages import HumanMessage, SystemMessage
from langchain_core.prompts import ChatPromptTemplate

from app.config import settings
from app.models import RunRequest, RunResponse, ToolCallRecord

logger = logging.getLogger(__name__)


def _get_llm(req: RunRequest) -> Any:
    """Resolve the LLM backend based on provider."""
    provider = req.provider or "ollama"
    model_name = req.model

    if provider == "openai":
        from langchain_openai import ChatOpenAI

        return ChatOpenAI(
            model=model_name or settings.openai_default_model,
            api_key=settings.openai_api_key,
            base_url=settings.openai_base_url,
            temperature=0.2,
        )

    # Default: Ollama (local-first).
    from langchain_ollama import ChatOllama

    ollama_url = req.config.get("ollama_base_url", settings.ollama_base_url)
    return ChatOllama(
        model=model_name or settings.ollama_default_model,
        base_url=ollama_url,
        temperature=0.2,
    )


def run(req: RunRequest) -> RunResponse:
    """Execute a LangChain prompt chain and return the result."""
    start = time.monotonic()

    try:
        llm = _get_llm(req)

        # Build a simple prompt chain.
        messages: list[Any] = []
        if req.system_prompt:
            messages.append(SystemMessage(content=req.system_prompt))
        messages.append(HumanMessage(content=req.prompt))

        # Check for template in config.
        template_str = req.config.get("template")
        if template_str:
            template_inputs = req.config.get("template_inputs", {})
            prompt = ChatPromptTemplate.from_template(template_str)
            chain = prompt | llm
            result = chain.invoke(template_inputs)
        else:
            result = llm.invoke(messages)

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
