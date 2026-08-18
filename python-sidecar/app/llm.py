"""LLM backend resolution, shared by every runner and streaming endpoint.

This used to live as three identical private ``_get_llm`` copies in
langchain_runner, langgraph_runner and stream_router. It lives here now for the
same reason gatewayauth exists once in the Go server: there must be exactly one
answer to "how does the sidecar reach a model backend".
"""

from __future__ import annotations

import time
from typing import Any

from app.config import settings
from app.models import RunRequest


def _gateway_headers() -> dict[str, str]:
    """Headers for an Ollama that sits behind the workstation auth gateway.

    Mirrors the Go server's gatewayauth package: a fresh HS256 JWT per request
    batch with claims {app, iat, exp}, presented bare in x-api-key. An empty
    OLLAMA_JWT_SECRET means "no gateway" and requests go out unsigned, which is
    correct for a direct Ollama.
    """
    if not settings.ollama_jwt_secret:
        return {}

    import jwt

    now = int(time.time())
    token = jwt.encode(
        {"app": "jobshout", "iat": now, "exp": now + 600},
        settings.ollama_jwt_secret,
        algorithm="HS256",
    )
    return {"x-api-key": token}


def get_llm(req: RunRequest) -> Any:
    """Resolve the LLM backend based on the request's provider."""
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
    kwargs: dict[str, Any] = {}
    headers = _gateway_headers()
    if headers:
        kwargs["client_kwargs"] = {"headers": headers}
    return ChatOllama(
        model=model_name or settings.ollama_default_model,
        base_url=ollama_url,
        temperature=0.2,
        **kwargs,
    )
