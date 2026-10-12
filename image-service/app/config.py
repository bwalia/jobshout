"""Configuration for the image service.

Read from the environment once at import. The service is a single process on a
single machine, so there is no reload path and nothing to invalidate.
"""

import os

# The model used when a request does not name one. Z-Image-Turbo is a few-step
# turbo model, so it answers in seconds rather than minutes, which is what makes
# generating a cover image an acceptable step inside an article run rather than
# something that has to happen out of band.
DEFAULT_MODEL = os.getenv("IMAGE_DEFAULT_MODEL", "z-image-turbo")

# Catalogue entries this service refuses to run, comma-separated. Empty — the
# default — means everything mflux offers is available.
#
# The catalogue answers what mflux can run and what is on this disk; it does not
# answer what this machine can afford to run. A model whose weights are cached
# still has to fit in the GPU beside whatever else the workstation is doing, and
# the largest entries evict the small turbo model that nearly every request
# actually wants. Naming one here leaves the code that loads it untouched, so
# the capability stays for a host with room for it, and only becomes unreachable
# on a host without.
#
# Matched on the exact catalogue name, so a family with more than one entry
# needs each listed: `qwen-image` (mflux's unquantised ~60 GB Qwen/Qwen-Image)
# and `qwen-image-2512` (the 4-bit repo in models.py) are two different
# downloads, and disabling either alone leaves the other reachable.
DISABLED_MODELS = frozenset(
    name.strip() for name in os.getenv("IMAGE_DISABLED_MODELS", "").split(",") if name.strip()
)

# Steps when a request names none. Unset — the normal case — means "ask the
# model", because the right count is a property of the model and not of the
# service: a turbo model converges in eight steps and a standard one needs
# around twenty, so any single number here is wrong for one of them. Setting it
# overrides every model at once, which is useful for a bulk experiment and
# nothing else.
_default_steps_raw = os.getenv("IMAGE_DEFAULT_STEPS", "").strip()
DEFAULT_STEPS = int(_default_steps_raw) if _default_steps_raw else None

# Ceilings. This service is one process in front of one GPU, so an unbounded
# step count or dimension is not merely a slow request — it is a denial of
# service against every other caller waiting on the lock.
MAX_STEPS = int(os.getenv("IMAGE_MAX_STEPS", "50"))
MAX_DIMENSION = int(os.getenv("IMAGE_MAX_DIMENSION", "1536"))

# Dimensions must be multiples of this. The VAE downsamples by 8 and the
# transformer patches by 2, so anything else is silently rounded somewhere deep
# in the model and comes back a different size than was asked for. Rejecting it
# up front is more honest than returning an image of unrequested dimensions.
DIMENSION_MULTIPLE = 16

# How long a request waits for the generation lock before giving up. Long,
# because the queue is expected to be short and a real generation takes tens of
# seconds; bounded, because a caller blocked forever is worse than one told to
# come back.
QUEUE_TIMEOUT_SECONDS = float(os.getenv("IMAGE_QUEUE_TIMEOUT", "600"))

# How many loaded models stay resident. One, by default, because a single image
# model peaks near the memory of the whole machine and a second resident copy is
# how this process gets OOM-killed.
MAX_LOADED_MODELS = max(1, int(os.getenv("IMAGE_MAX_LOADED_MODELS", "1")))

# Optional quantisation applied at load (3/4/5/6/8). Trades image fidelity for
# memory. Unset means load at the model's native precision.
_quantize_raw = os.getenv("IMAGE_QUANTIZE", "").strip()
QUANTIZE = int(_quantize_raw) if _quantize_raw else None

# Shared secret for the gateway JWT. Unset means the service authenticates
# nothing — see auth.py for why that is the documented local-development
# behaviour rather than an oversight.
JWT_SECRET = os.getenv("IMAGE_JWT_SECRET", "")

# Slack allowed when checking the token's time claims, in seconds. The caller
# mints `iat` by truncating its clock down to a whole second, so a verifier
# whose own clock is a few milliseconds behind sees an `iat` in the future and
# rejects a perfectly good token. Two NTP-synced machines are close enough for
# that to happen intermittently and nothing else, which is the worst kind of
# failure to debug. A minute is far below the ten-minute token lifetime, so it
# costs nothing worth having.
JWT_LEEWAY = int(os.getenv("IMAGE_JWT_LEEWAY", "60"))

# ─── Video (see video.py) ─────────────────────────────────────────────────────
# The video tools live in their own virtualenvs, installed by
# install-video-tools.sh. Each defaults to where that script puts it, so a host
# set up by the script needs none of these. A missing tool switches its stage
# off rather than failing the service: no LTX means stills with a slow camera
# move, no TTS means no video at all (narration sets every shot's length).
_TOOLS = os.path.expanduser(os.getenv("VIDEO_TOOLS_DIR", "~/.jobshout/media-tools"))
VIDEO_DIR = os.path.expanduser(os.getenv("VIDEO_DIR", "~/.jobshout/videos"))
VIDEO_FFMPEG = os.getenv("VIDEO_FFMPEG", "/opt/homebrew/opt/ffmpeg-full/bin/ffmpeg")
VIDEO_FFPROBE = os.getenv("VIDEO_FFPROBE", "/opt/homebrew/opt/ffmpeg-full/bin/ffprobe")
VIDEO_TTS_BIN = os.getenv("VIDEO_TTS_BIN", f"{_TOOLS}/tts/.venv/bin/mlx_audio.tts.generate")
VIDEO_TTS_MODEL = os.getenv("VIDEO_TTS_MODEL", "mlx-community/Kokoro-82M-bf16")
VIDEO_DEFAULT_VOICE = os.getenv("VIDEO_DEFAULT_VOICE", "af_heart")
VIDEO_LTX_BIN = os.getenv("VIDEO_LTX_BIN", f"{_TOOLS}/ltx-2-mlx/.venv/bin/ltx-2-mlx")
# LTX-2.3 because its weights are ungated. LTX-2.5 (dgrauet/ltx-2.5-mlx-q8) is
# the better model once a Hugging Face login on this host has accepted its
# licence and the weights are downloaded — then set this, nothing else changes.
VIDEO_LTX_MODEL = os.getenv("VIDEO_LTX_MODEL", "dgrauet/ltx-2.3-mlx-q8")
# Extra ltx-2-mlx flags, space-separated: the pipeline choice lives here so a
# quality/speed trade can be made without a deploy.
VIDEO_LTX_ARGS = os.getenv("VIDEO_LTX_ARGS", "--distilled").split()
VIDEO_KEYFRAME_MODEL = os.getenv("VIDEO_KEYFRAME_MODEL", DEFAULT_MODEL)
# Longest single animated clip, in frames at 24 fps. LTX wants 8n+1 frames, so
# whatever is set is rounded down onto that grid. 121 ≈ 5 s; a longer narration
# holds the clip's last frame for the remainder rather than paying for frames
# the model renders slowly and less coherently.
_max_frames = max(25, int(os.getenv("VIDEO_MAX_CLIP_FRAMES", "121")))
VIDEO_MAX_CLIP_FRAMES = (_max_frames - 1) // 8 * 8 + 1
VIDEO_CLIP_TIMEOUT = int(os.getenv("VIDEO_CLIP_TIMEOUT", "1800"))
VIDEO_MAX_SHOTS = int(os.getenv("VIDEO_MAX_SHOTS", "30"))
# Jobs waiting behind the one rendering. Each is up to an hour of GPU, so a
# long queue is a day of work nobody is watching.
VIDEO_MAX_QUEUE = int(os.getenv("VIDEO_MAX_QUEUE", "4"))
# A job interrupted by a restart is retried; one that keeps taking the service
# down with it is not.
VIDEO_MAX_ATTEMPTS = int(os.getenv("VIDEO_MAX_ATTEMPTS", "2"))
VIDEO_KEEP_DAYS = float(os.getenv("VIDEO_KEEP_DAYS", "7"))

HOST = os.getenv("IMAGE_HOST", "0.0.0.0")

# 11435 sits one above Ollama's 11434 — deliberately adjacent, so the two
# workstation model services are obviously a pair.
PORT = int(os.getenv("IMAGE_PORT", "11435"))

LOG_LEVEL = os.getenv("IMAGE_LOG_LEVEL", "info")
