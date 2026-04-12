"""JobShout Python Sidecar — LangChain + LangGraph execution service."""

import logging

from fastapi import FastAPI

from app.config import settings
from app.routers.health import router as health_router
from app.routers.langchain_router import router as langchain_router
from app.routers.langgraph_router import router as langgraph_router
from app.routers.stream_router import router as stream_router

# Configure logging.
logging.basicConfig(
    level=getattr(logging, settings.log_level.upper(), logging.INFO),
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)

app = FastAPI(
    title="JobShout Python Sidecar",
    description="LangChain and LangGraph execution service for the JobShout platform.",
    version="0.2.0",
)

app.include_router(health_router)
app.include_router(langchain_router, prefix="/run/langchain", tags=["langchain"])
app.include_router(langgraph_router, prefix="/run/langgraph", tags=["langgraph"])
app.include_router(stream_router, prefix="/stream", tags=["streaming"])


@app.get("/")
async def root():
    return {
        "service": "jobshout-python-sidecar",
        "engines": ["langchain", "langgraph"],
        "streaming": True,
        "plugins": True,
        "ollama_url": settings.ollama_base_url,
    }
