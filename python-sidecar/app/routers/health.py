"""Health check router."""

from fastapi import APIRouter

router = APIRouter()


@router.get("/health")
async def health():
    return {"status": "ok", "service": "jobshout-python-sidecar"}
