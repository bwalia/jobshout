"""The model catalogue: what mflux can run, and what is actually on this disk.

Callers need both facts. A model mflux knows about but whose weights are not
cached will "work" — it will also spend twenty minutes downloading tens of
gigabytes first, which is not the same kind of working, and a picker that
presented the two identically would be lying to whoever used it.
"""

import logging
import os
from dataclasses import dataclass
from pathlib import Path

from app import config

logger = logging.getLogger(__name__)

# Models that take an input image rather than only a prompt (editing, upscaling,
# depth and controlnet variants). They are real mflux capabilities, but this
# service exposes text-to-image only, so they are filtered out of the catalogue
# rather than offered and then rejected at generate time.
_NON_TEXT_TO_IMAGE_MARKERS = ("edit", "kontext", "redux", "depth", "controlnet", "fill", "seedvr2")

# Weights below this are a stub, not a model. Hugging Face leaves a directory
# behind holding only `refs/` after a metadata-only touch — FLUX.1-schnell is in
# exactly that state on the workstation — and a bare directory check would
# report it as present.
_MIN_REAL_WEIGHTS_BYTES = 100 * 1024 * 1024

# Steps that reach a usable image. A turbo model converges in single digits; a
# standard one needs an order of magnitude more, and running one at the other's
# count is not a slightly worse picture but an unusable one — eight steps of a
# standard model is noise, and twenty of a turbo model is twenty seconds spent
# to no effect. So the default follows the model rather than the service.
_TURBO_STEPS = 8
_STANDARD_STEPS = 20


@dataclass(frozen=True)
class ImageModel:
    """One entry in the catalogue."""

    name: str
    repo: str
    # downloaded is what separates "generates in seconds" from "downloads 31 GB
    # first". It is recomputed per request rather than cached: weights arrive
    # while the service is running, and a stale "no" is a picker that never
    # notices the model it just pulled.
    downloaded: bool
    # turbo models reach a usable image in single-digit steps. Surfaced so a
    # caller choosing on its own behalf can prefer one without hard-coding a
    # list of names.
    turbo: bool
    # What this model gets when a request names no step count. Reported rather
    # than kept private because a caller that wants to raise or lower it needs
    # to know what it is raising or lowering from.
    default_steps: int


@dataclass(frozen=True)
class CustomModel:
    """A model mflux can run but does not list."""

    name: str
    repo: str
    # The mflux family whose architecture loads this repo — the `--base-model`
    # argument. A third-party repo carries weights but not the knowledge of how
    # to assemble them, which is what this supplies.
    base_model: str
    turbo: bool
    steps: int
    # Bits the published repo is *already* quantised to, or None if it ships at
    # native precision. Quantising an already-quantised tensor on load is not a
    # smaller model, it is a broken one, so this tells the loader to leave
    # IMAGE_QUANTIZE unapplied.
    quantized_bits: int | None


# Models that run on mflux but live in a third-party Hugging Face repo rather
# than in mflux's own table. On the command line they are reached with
# `--model <org/repo> --base-model <family>`; this is that same pair, given a
# name so nothing upstream has to know the repo path.
#
# `qwen-image-2512` is kept distinct from mflux's built-in `qwen-image` rather
# than overriding it, because the two are genuinely different downloads:
# built-in `qwen-image` is `Qwen/Qwen-Image`, unquantised and around 60 GB, and
# is not on this disk. Serving both under one name would mean a request either
# generating in a minute or downloading for an hour, with nothing in the request
# to say which — the distinction the `downloaded` flag exists to make visible.
_CUSTOM_MODELS: tuple[CustomModel, ...] = (
    CustomModel(
        name="qwen-image-2512",
        repo="mlx-community/Qwen-Image-2512-4bit",
        base_model="qwen",
        turbo=False,
        steps=_STANDARD_STEPS,
        quantized_bits=4,
    ),
)


def _hf_cache_root() -> Path:
    """Where Hugging Face keeps its hub cache, honouring the usual overrides."""
    if env := os.getenv("HF_HUB_CACHE"):
        return Path(env)
    if env := os.getenv("HF_HOME"):
        return Path(env) / "hub"
    return Path.home() / ".cache" / "huggingface" / "hub"


def _is_downloaded(repo: str) -> bool:
    """Report whether `repo`'s weights are really on this disk.

    "Really" is doing work here: the presence of the cache directory proves
    only that something touched it. What proves the weights exist is a snapshot
    holding a non-trivial amount of data, so that is what gets measured.
    """
    folder = _hf_cache_root() / ("models--" + repo.replace("/", "--"))
    snapshots = folder / "snapshots"
    if not snapshots.is_dir():
        return False

    total = 0
    for path in snapshots.rglob("*"):
        try:
            # Snapshot entries are symlinks into blobs/; stat() follows them,
            # which is what we want — the question is whether the bytes exist,
            # not whether the link does.
            if path.is_file():
                total += path.stat().st_size
                if total >= _MIN_REAL_WEIGHTS_BYTES:
                    return True
        except OSError:
            # A dangling link means that blob was evicted. It is not downloaded;
            # skipping it is the correct contribution to the total.
            continue
    return False


def _is_turbo(name: str) -> bool:
    return "turbo" in name or "schnell" in name


def catalogue() -> list[ImageModel]:
    """Every text-to-image model this service will run, with local availability.

    Downloaded models sort first, then turbo models, then by name — so the entry
    a caller most likely wants is the one at the top of the list.

    Entries named in IMAGE_DISABLED_MODELS are omitted. Filtering here rather
    than at the request handler is what makes the setting airtight: `is_known()`
    reads this list, so a disabled model is missing from GET /api/models *and*
    rejected by POST /api/generate, instead of being merely hidden from the
    picker while still loadable by anyone who names it directly.
    """
    from mflux.models.common.config.model_config import AVAILABLE_MODELS

    out: list[ImageModel] = []
    for name, cfg in AVAILABLE_MODELS.items():
        if name in config.DISABLED_MODELS:
            continue
        if any(marker in name for marker in _NON_TEXT_TO_IMAGE_MARKERS):
            continue
        repo = cfg.model_name
        turbo = _is_turbo(name)
        out.append(
            ImageModel(
                name=name,
                repo=repo,
                downloaded=_is_downloaded(repo),
                turbo=turbo,
                default_steps=_TURBO_STEPS if turbo else _STANDARD_STEPS,
            )
        )

    out.extend(
        ImageModel(
            name=custom.name,
            repo=custom.repo,
            downloaded=_is_downloaded(custom.repo),
            turbo=custom.turbo,
            default_steps=custom.steps,
        )
        for custom in _CUSTOM_MODELS
        if custom.name not in config.DISABLED_MODELS
    )

    out.sort(key=lambda m: (not m.downloaded, not m.turbo, m.name))
    return out


def custom(name: str) -> CustomModel | None:
    """The third-party repo behind `name`, or None if mflux lists it itself."""
    return next((m for m in _CUSTOM_MODELS if m.name == name), None)


def default_steps(name: str) -> int:
    """Steps for `name` when a request does not ask for a count.

    Derived from the name rather than read out of `catalogue()`, because that
    walks the Hugging Face cache to answer a question about downloads that this
    one does not need to ask.
    """
    if (entry := custom(name)) is not None:
        return entry.steps
    return _TURBO_STEPS if _is_turbo(name) else _STANDARD_STEPS


def is_known(name: str) -> bool:
    """Report whether `name` is a text-to-image model this service will run."""
    return any(m.name == name for m in catalogue())
