"""JobShout Image Service — mflux behind HTTP.

See README.md for why this runs on the workstation rather than in the cluster.
"""

import asyncio
import logging
import random
from typing import Literal

from fastapi import Depends, FastAPI, HTTPException, Response, status
from fastapi.responses import FileResponse
from pydantic import BaseModel, Field

# models is needed for default_steps below. It is safe to call from the request
# thread — unlike catalogue(), it derives the count from the model name and never
# touches mflux or the Hugging Face cache, so it does not belong on the dedicated
# MLX thread that the rest of this module now routes work to.
from app import config, models, video
from app.auth import require_auth
from app.generator import GenerationBusy, UnknownModel, generator

logging.basicConfig(
    level=getattr(logging, config.LOG_LEVEL.upper(), logging.INFO),
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)

app = FastAPI(
    title="JobShout Image Service",
    description="Text-to-image generation for the JobShout platform, backed by mflux on Apple MLX.",
    version="1.0.0",
)


class GenerateRequest(BaseModel):
    prompt: str = Field(min_length=1, max_length=4000)
    model: str = ""
    width: int = 1024
    height: int = 1024
    steps: int = 0
    # A seed of -1 means "choose one". The chosen value comes back in the
    # response, because a caller that cannot learn the seed cannot reproduce the
    # image, and reproducing it is the only way to regenerate a cover that was
    # right the first time.
    seed: int = -1
    guidance: float | None = None
    negative_prompt: str = ""


class GenerateResponse(BaseModel):
    image_base64: str
    model: str
    seed: int
    width: int
    height: int
    steps: int
    duration_ms: int


class ModelEntry(BaseModel):
    name: str
    repo: str
    downloaded: bool
    turbo: bool
    default_steps: int


class ModelsResponse(BaseModel):
    models: list[ModelEntry]
    default: str


@app.get("/health")
async def health():
    """Liveness only.

    Deliberately does not load or touch a model. A health check that fails
    because the GPU is busy is a health check that reports "down" during normal
    operation, and one that loads a model to prove it can is a health check that
    costs 30 GB.
    """
    return {"status": "ok", "service": "jobshout-image-service"}


@app.get("/")
async def root():
    return {
        "service": "jobshout-image-service",
        "backend": "mflux",
        "default_model": config.DEFAULT_MODEL,
        "authenticated": bool(config.JWT_SECRET),
    }


@app.get("/api/models", response_model=ModelsResponse)
async def list_models(caller: str = Depends(require_auth)):
    """The catalogue, with local availability per model."""
    # Through the generator so the mflux import shares the dedicated MLX
    # thread — same affinity rule as generate, otherwise listing models can
    # initialise Metal on a random pool worker.
    entries = await asyncio.to_thread(generator.catalogue)
    return ModelsResponse(
        models=[ModelEntry(**vars(m)) for m in entries],
        default=config.DEFAULT_MODEL,
    )


def _validate_dimension(value: int, label: str) -> int:
    if value <= 0:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, f"{label} must be positive")
    if value > config.MAX_DIMENSION:
        raise HTTPException(
            status.HTTP_400_BAD_REQUEST,
            f"{label} {value} exceeds the maximum of {config.MAX_DIMENSION}",
        )
    if value % config.DIMENSION_MULTIPLE != 0:
        raise HTTPException(
            status.HTTP_400_BAD_REQUEST,
            f"{label} must be a multiple of {config.DIMENSION_MULTIPLE}",
        )
    return value


@app.post("/api/generate", response_model=GenerateResponse)
async def generate(req: GenerateRequest, caller: str = Depends(require_auth)):
    model_name = req.model.strip() or config.DEFAULT_MODEL

    width = _validate_dimension(req.width, "width")
    height = _validate_dimension(req.height, "height")

    # An unasked-for step count falls to the model's own, not to one number for
    # every model — eight steps is right for a turbo model and noise from a
    # standard one. IMAGE_DEFAULT_STEPS, when set, overrides both.
    steps = req.steps if req.steps > 0 else (config.DEFAULT_STEPS or models.default_steps(model_name))
    if steps > config.MAX_STEPS:
        raise HTTPException(
            status.HTTP_400_BAD_REQUEST,
            f"steps {steps} exceeds the maximum of {config.MAX_STEPS}",
        )

    # randrange rather than a fixed default: repeating one seed across every
    # unseeded request would make every article's cover the same picture.
    seed = req.seed if req.seed >= 0 else random.randrange(0, 2**31 - 1)

    logger.info(
        "generate: caller=%s model=%s %dx%d steps=%d seed=%d",
        caller, model_name, width, height, steps, seed,
    )

    try:
        # Off the event loop so /health stays responsive during a long
        # generate. generator.generate itself re-dispatches MLX load/infer
        # onto a single dedicated worker thread — required because MLX GPU
        # streams are thread-local and asyncio.to_thread's default pool
        # would otherwise reuse a cached model from the wrong thread.
        result = await asyncio.to_thread(
            generator.generate,
            prompt=req.prompt,
            model_name=model_name,
            width=width,
            height=height,
            steps=steps,
            seed=seed,
            guidance=req.guidance,
            negative_prompt=req.negative_prompt,
        )
    except UnknownModel as exc:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, str(exc))
    except GenerationBusy as exc:
        # 503 with Retry-After, not 500: the request was fine and trying again
        # later is the correct response to it.
        raise HTTPException(
            status.HTTP_503_SERVICE_UNAVAILABLE,
            str(exc),
            headers={"Retry-After": "30"},
        )
    except Exception as exc:  # noqa: BLE001 — the boundary turns anything into a 500
        logger.exception("generation failed")
        raise HTTPException(status.HTTP_500_INTERNAL_SERVER_ERROR, f"generation failed: {exc}")

    return GenerateResponse(**vars(result))


# ─── Video ───────────────────────────────────────────────────────────────────
# Same process as images on purpose: one GPU, one lock, one gateway host. See
# video.py for the pipeline.


class VideoShot(BaseModel):
    narration: str = Field(min_length=1, max_length=800)
    visual: str = Field(min_length=1, max_length=2000)


class VideoRequest(BaseModel):
    # Caller-chosen, so a retried submission after a dropped response finds the
    # job it already started instead of rendering the video twice.
    id: str = ""
    title: str = Field(default="", max_length=200)
    shots: list[VideoShot] = Field(min_length=1)
    format: Literal["landscape", "portrait"] = "landscape"
    voice: str = ""
    style: str = Field(default="", max_length=500)
    motion: bool = True
    captions: bool = True
    seed: int = -1
    # Opaque to this service and handed back on every read: lets a caller that
    # restarted find which of its tasks a job belongs to.
    ref: dict[str, str] = Field(default_factory=dict)


@app.on_event("startup")
async def start_video_worker() -> None:
    # Video is the newer, optional half of this service. A disk it cannot
    # write to, or a corrupt job file, must not take image generation down
    # with it — log, and serve images without video.
    try:
        video.renderer.start()
    except Exception:  # noqa: BLE001
        logger.exception("video worker failed to start; serving images only")


@app.get("/api/videos/capabilities")
async def video_capabilities(caller: str = Depends(require_auth)):
    return video.capabilities()


@app.post("/api/videos", status_code=status.HTTP_202_ACCEPTED)
async def submit_video(req: VideoRequest, response: Response, caller: str = Depends(require_auth)):
    if req.id and not video.valid_id(req.id):
        raise HTTPException(status.HTTP_400_BAD_REQUEST, "id must be 8–64 letters, digits or dashes")
    if req.voice and not video.valid_voice(req.voice):
        raise HTTPException(status.HTTP_400_BAD_REQUEST, f"unknown voice {req.voice!r}")
    if len(req.shots) > config.VIDEO_MAX_SHOTS:
        raise HTTPException(status.HTTP_400_BAD_REQUEST, f"at most {config.VIDEO_MAX_SHOTS} shots per video")
    try:
        job, created = video.renderer.submit(req.model_dump())
    except video.QueueFull as exc:
        raise HTTPException(status.HTTP_503_SERVICE_UNAVAILABLE, str(exc), headers={"Retry-After": "300"})
    if not created:
        response.status_code = status.HTTP_200_OK
    logger.info("video %s: caller=%s shots=%d created=%s", job["id"], caller, len(req.shots), created)
    return video.public(job)


@app.get("/api/videos")
async def list_videos(caller: str = Depends(require_auth)):
    return {"videos": [video.public(j) for j in video.renderer.list()]}


@app.get("/api/videos/{job_id}")
async def get_video(job_id: str, caller: str = Depends(require_auth)):
    job = video.renderer.get(job_id)
    if job is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "no such video")
    return video.public(job)


@app.delete("/api/videos/{job_id}")
async def cancel_video(job_id: str, caller: str = Depends(require_auth)):
    """Cancel a render (queued: at once; running: at its next step)."""
    job = video.renderer.cancel(job_id)
    if job is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "no such video")
    logger.info("video %s: cancel requested by %s (status %s)", job_id, caller, job["status"])
    return video.public(job)


class ClaimRequest(BaseModel):
    holder: str = Field(min_length=1, max_length=100)
    lease_seconds: float = Field(default=900, gt=0, le=3600)


@app.post("/api/videos/{job_id}/claim")
async def claim_video(job_id: str, req: ClaimRequest, caller: str = Depends(require_auth)):
    """Let exactly one caller collect a finished video (see VideoRenderer.claim)."""
    granted = video.renderer.claim(job_id, req.holder, req.lease_seconds)
    if granted is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "no finished video with that id")
    if not granted:
        raise HTTPException(status.HTTP_409_CONFLICT, "another caller is collecting this video")
    return {"claimed": True}


@app.get("/api/videos/{job_id}/file")
async def video_file(job_id: str, caller: str = Depends(require_auth)):
    path = video.renderer.file(job_id)
    if path is None:
        raise HTTPException(status.HTTP_404_NOT_FOUND, "video is not ready")
    return FileResponse(path, media_type="video/mp4", filename=f"{job_id}.mp4")
