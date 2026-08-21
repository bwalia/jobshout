"""Model loading and image generation.

Three things make this more than a thin wrapper around mflux:

* **Residency.** Loading a 27 B-parameter model off disk costs several seconds
  and most of the machine's memory. Loading it once per request would dominate
  the response time of every request, so loaded models are kept.
* **Serialisation.** One image peaks near 27 GB. Two concurrently do not fit,
  and the failure is not a slow answer but an out-of-memory kill that takes the
  process down and fails every queued request with it. So generation runs one at
  a time, behind a lock.
* **Thread affinity.** MLX GPU streams are thread-local: a stream created while
  loading a model on thread A cannot be used from thread B. Dispatching each
  request through `asyncio.to_thread` (the default multi-worker pool) therefore
  fails intermittently with `There is no Stream(gpu, N) in current thread` once
  the model is cached. All load and generate work is pinned to one dedicated
  worker thread for the lifetime of the process.
"""

import base64
import concurrent.futures
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
    from mflux.models.common.config.model_config import AVAILABLE_MODELS, ModelConfig

    from app import models as catalogue

    # A third-party repo is absent from mflux's table, so it is resolved from
    # the repo path plus the family whose architecture loads it — the same pair
    # `mflux-generate-qwen --model org/repo --base-model qwen` takes.
    if (custom := catalogue.custom(name)) is not None:
        return ModelConfig.from_name(custom.repo, base_model=custom.base_model)

    cfg = AVAILABLE_MODELS.get(name)
    if cfg is None:
        raise UnknownModel(f"unknown image model: {name}")
    return cfg


class Generator:
    """Owns the loaded models, the GPU lock, and the dedicated MLX thread."""

    def __init__(self) -> None:
        # OrderedDict as an LRU: generation moves a model to the end, so the
        # entry evicted when the cache is full is the one least recently used.
        self._loaded: "OrderedDict[str, object]" = OrderedDict()
        # Guards _loaded. Held only for bookkeeping, never across a generation —
        # that is what _gpu is for.
        self._registry_lock = threading.Lock()
        # Guards the GPU itself. Everything that allocates model-sized memory —
        # loading and generating alike — holds this. Acquired on the caller
        # thread so IMAGE_QUEUE_TIMEOUT still bounds queue wait; the actual MLX
        # work then runs on _mlx_executor.
        self._gpu = threading.Lock()
        # One worker for the process lifetime. MLX streams are thread-local; a
        # multi-worker pool is exactly what produces Stream(gpu, N) errors when
        # a cached model is reused from a different thread than the one that
        # loaded it.
        self._mlx_executor = concurrent.futures.ThreadPoolExecutor(
            max_workers=1,
            thread_name_prefix="mlx-gpu",
        )

    def _load(self, name: str):
        """Load `name`, evicting whatever no longer fits. Caller holds _gpu and runs on the MLX thread."""
        from app import models as catalogue

        cfg = _model_config(name)
        cls = _variant_class(name)

        # A repo published pre-quantised is already as small as it gets.
        # Quantising it again on load does not halve it a second time; it
        # re-quantises tensors that have already lost that precision, so the
        # service's own setting is dropped rather than applied twice.
        quantize = config.QUANTIZE
        custom = catalogue.custom(name)
        if custom is not None and custom.quantized_bits is not None:
            if quantize is not None:
                logger.info(
                    "ignoring IMAGE_QUANTIZE=%d for %s — the repo ships at %d-bit",
                    quantize, name, custom.quantized_bits,
                )
            quantize = None

        # Evict before loading, not after. Loading first would briefly hold both
        # the outgoing and incoming model in memory, which is exactly the state
        # this cache exists to avoid.
        with self._registry_lock:
            while len(self._loaded) >= config.MAX_LOADED_MODELS:
                evicted, _ = self._loaded.popitem(last=False)
                logger.info("evicting model %s to make room for %s", evicted, name)

        logger.info("loading model %s (%s)", name, cfg.model_name)
        started = time.monotonic()
        model = cls(model_config=cfg, quantize=quantize)
        logger.info("loaded %s in %.1fs", name, time.monotonic() - started)

        with self._registry_lock:
            self._loaded[name] = model
        return model

    def _acquire_gpu(self) -> None:
        if not self._gpu.acquire(timeout=config.QUEUE_TIMEOUT_SECONDS):
            raise GenerationBusy(
                f"image generation is busy — waited {config.QUEUE_TIMEOUT_SECONDS:.0f}s for the GPU"
            )

    def _generate_on_mlx_thread(
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
        started: float,
    ) -> GenerationResult:
        """Run load + generate on the dedicated MLX thread. Caller holds _gpu."""
        from app import models as catalogue

        # Catalogue / AVAILABLE_MODELS import mflux. Doing that here — not on a
        # random asyncio pool thread — keeps Metal initialisation on the same
        # thread that will own the model's streams.
        if not catalogue.is_known(model_name):
            raise UnknownModel(f"unknown image model: {model_name}")

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
        """Generate one image. Blocks until the GPU is free.

        The queue lock is taken on the calling thread so waiters still honour
        IMAGE_QUEUE_TIMEOUT. MLX work always runs on the dedicated worker.
        """
        started = time.monotonic()
        self._acquire_gpu()
        try:
            return self._mlx_executor.submit(
                self._generate_on_mlx_thread,
                prompt=prompt,
                model_name=model_name,
                width=width,
                height=height,
                steps=steps,
                seed=seed,
                guidance=guidance,
                negative_prompt=negative_prompt,
                started=started,
            ).result()
        finally:
            self._gpu.release()

    def catalogue(self):
        """List models on the MLX thread so mflux import shares that affinity."""
        from app import models as catalogue

        return self._mlx_executor.submit(catalogue.catalogue).result()


# One generator per process, because there is one GPU per process.
generator = Generator()
