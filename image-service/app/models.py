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


def catalogue() -> list[ImageModel]:
    """Every text-to-image model mflux offers, with local availability.

    Downloaded models sort first, then turbo models, then by name — so the entry
    a caller most likely wants is the one at the top of the list.
    """
    from mflux.models.common.config.model_config import AVAILABLE_MODELS

    out: list[ImageModel] = []
    for name, cfg in AVAILABLE_MODELS.items():
        if any(marker in name for marker in _NON_TEXT_TO_IMAGE_MARKERS):
            continue
        repo = cfg.model_name
        out.append(
            ImageModel(
                name=name,
                repo=repo,
                downloaded=_is_downloaded(repo),
                turbo="turbo" in name or "schnell" in name,
            )
        )

    out.sort(key=lambda m: (not m.downloaded, not m.turbo, m.name))
    return out


def is_known(name: str) -> bool:
    """Report whether `name` is a text-to-image model this service will run."""
    return any(m.name == name for m in catalogue())
