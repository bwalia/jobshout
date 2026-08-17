"""Model loading and image generation.

Two things make this more than a thin wrapper around mflux:

* **Residency.** Loading a 27 B-parameter model off disk costs several seconds
  and most of the machine's memory. Loading it once per request would dominate
  the response time of every request, so loaded models are kept.
* **Serialisation.** One image peaks near 27 GB. Two concurrently do not fit,
  and the failure is not a slow answer but an out-of-memory kill that takes the
  process down and fails every queued request with it. So generation runs one at
  a time, behind a lock.
"""

import base64
import inspect
import io
import logging
import threading
import time
from collections import OrderedDict
from dataclasses import dataclass

from app import config

logger = logging.getLogger(__name__)


class GenerationBusy(Exception):
    """Raised when a request waited for the lock longer than it is allowed to."""


class UnknownModel(Exception):
    """Raised when a request names a model this service will not run."""


@dataclass
class GenerationResult:
    image_base64: str
    model: str
    seed: int
    width: int
    height: int
    steps: int
    duration_ms: int


def _variant_class(name: str):
    """Map an mflux model name onto the class that runs it.

    mflux exposes one class per architecture rather than a single dispatching
    entry point, so this table is the dispatch. Imports are deferred into the
    branches: each pulls in its own transformer and text-encoder modules, and
    importing all eight to serve one would cost startup time for nothing.
    """
    if name.startswith("z-image"):
        from mflux.models.z_image.variants.z_image import ZImage

        return ZImage
    if name.startswith("flux2-klein"):
        from mflux.models.flux2.variants.txt2img.flux2_klein import Flux2Klein

        return Flux2Klein
    if name.startswith("qwen-image"):
        from mflux.models.qwen.variants.txt2img.qwen_image import QwenImage

        return QwenImage
    if name == "krea-2":
        from mflux.models.krea2.variants.txt2img.krea2 import Krea2

        return Krea2
    if name.startswith("fibo"):
        from mflux.models.fibo.variants.txt2img.fibo import FIBO

        return FIBO
    if name.startswith("ernie-image"):
        from mflux.models.ernie_image.variants.txt2img.ernie_image import ErnieImage

        return ErnieImage
    if name.startswith("ideogram"):
        from mflux.models.ideogram4.variants.txt2img.ideogram4 import Ideogram4

        return Ideogram4
    # The FLUX.1 family (dev, schnell, krea-dev) all run on Flux1. It is last
    # because its names are the ones without a distinguishing prefix.
    from mflux.models.flux.variants.txt2img.flux import Flux1

    return Flux1


def _model_config(name: str):
    """Resolve an mflux ModelConfig from a model name."""
    from mflux.models.common.config.model_config import AVAILABLE_MODELS

    cfg = AVAILABLE_MODELS.get(name)
    if cfg is None:
        raise UnknownModel(f"unknown image model: {name}")
    return cfg


class Generator:
    """Owns the loaded models and the one lock that guards the GPU."""

    def __init__(self) -> None:
        # OrderedDict as an LRU: generation moves a model to the end, so the
        # entry evicted when the cache is full is the one least recently used.
        self._loaded: "OrderedDict[str, object]" = OrderedDict()
        # Guards _loaded. Held only for bookkeeping, never across a generation —
        # that is what _gpu is for.
        self._registry_lock = threading.Lock()
        # Guards the GPU itself. Everything that allocates model-sized memory —
        # loading and generating alike — holds this.
        self._gpu = threading.Lock()

    def _load(self, name: str):
        """Load `name`, evicting whatever no longer fits. Caller holds _gpu."""
        cfg = _model_config(name)
        cls = _variant_class(name)

        # Evict before loading, not after. Loading first would briefly hold both
        # the outgoing and incoming model in memory, which is exactly the state
        # this cache exists to avoid.
        with self._registry_lock:
            while len(self._loaded) >= config.MAX_LOADED_MODELS:
                evicted, _ = self._loaded.popitem(last=False)
                logger.info("evicting model %s to make room for %s", evicted, name)

        logger.info("loading model %s (%s)", name, cfg.model_name)
        started = time.monotonic()
        model = cls(model_config=cfg, quantize=config.QUANTIZE)
        logger.info("loaded %s in %.1fs", name, time.monotonic() - started)

        with self._registry_lock:
            self._loaded[name] = model
        return model

    def _acquire_gpu(self) -> None:
        if not self._gpu.acquire(timeout=config.QUEUE_TIMEOUT_SECONDS):
            raise GenerationBusy(
                f"image generation is busy — waited {config.QUEUE_TIMEOUT_SECONDS:.0f}s for the GPU"
            )

    def generate(
        self,
        *,
        prompt: str,
        model_name: str,
        width: int,
        height: int,
        steps: int,
        seed: int,
        guidance: float | None,
        negative_prompt: str,
    ) -> GenerationResult:
        """Generate one image. Blocks until the GPU is free."""
        from app import models as catalogue

        if not catalogue.is_known(model_name):
            raise UnknownModel(f"unknown image model: {model_name}")

        started = time.monotonic()
        self._acquire_gpu()
        try:
            with self._registry_lock:
                model = self._loaded.get(model_name)
                if model is not None:
                    self._loaded.move_to_end(model_name)
            if model is None:
                model = self._load(model_name)

            # Not every variant takes every argument — Flux2Klein has no
            # negative_prompt, and guidance is meaningless on models that do not
            # support it. Passing one anyway is a TypeError at the worst moment,
            # so the signature decides what gets sent.
            accepted = inspect.signature(model.generate_image).parameters
            kwargs: dict = {
                "seed": seed,
                "prompt": prompt,
                "num_inference_steps": steps,
                "width": width,
                "height": height,
            }
            if guidance is not None and "guidance" in accepted:
                kwargs["guidance"] = guidance
            if negative_prompt and "negative_prompt" in accepted:
                kwargs["negative_prompt"] = negative_prompt

            result = model.generate_image(**kwargs)
        finally:
            self._gpu.release()

        # mflux returns a GeneratedImage wrapping a PIL image from most variants
        # and, depending on version, the bare PIL image from others. Both are
        # handled rather than depending on which one this build returns.
        image = getattr(result, "image", result)

        buffer = io.BytesIO()
        image.save(buffer, format="PNG")
        encoded = base64.b64encode(buffer.getvalue()).decode("ascii")

        return GenerationResult(
            image_base64=encoded,
            model=model_name,
            seed=seed,
            width=width,
            height=height,
            steps=steps,
            duration_ms=int((time.monotonic() - started) * 1000),
        )


# One generator per process, because there is one GPU per process.
generator = Generator()
