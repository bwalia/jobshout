"""LangChain execution endpoint."""

import logging

from fastapi import APIRouter, Depends, HTTPException, Request

from app.models import RunRequest, RunResponse
from app.runners import langchain_runner

logger = logging.getLogger(__name__)

router = APIRouter()


def _verify_secret(request: Request) -> None:
    """Validate the sidecar shared secret."""
    from app.config import settings

    secret = request.headers.get("X-Sidecar-Secret", "")
    if secret != settings.sidecar_secret:
        raise HTTPException(status_code=401, detail="Invalid sidecar secret")


@router.post("", response_model=RunResponse)
async def run_langchain(req: RunRequest, _: None = Depends(_verify_secret)):
    """Execute a LangChain chain/agent."""
    logger.info(
        "LangChain execution request",
        extra={"execution_id": req.execution_id, "agent_id": req.agent_id},
    )
    return langchain_runner.run(req)
