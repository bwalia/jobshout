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

HOST = os.getenv("IMAGE_HOST", "0.0.0.0")

# 11435 sits one above Ollama's 11434 — deliberately adjacent, so the two
# workstation model services are obviously a pair.
PORT = int(os.getenv("IMAGE_PORT", "11435"))

LOG_LEVEL = os.getenv("IMAGE_LOG_LEVEL", "info")
