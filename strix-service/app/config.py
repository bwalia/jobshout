"""Configuration for the pentest service.

Read from the environment once at import. The service is a single process on a
single machine, so there is no reload path and nothing to invalidate.

Names deliberately echo the image service's (``IMAGE_*`` → ``STRIX_*``): the
platform now runs three workstation services behind one arrangement, and a
third set of conventions would be a third thing to remember under pressure.
"""

import os
from pathlib import Path

# ─── The scanner ────────────────────────────────────────────────────────────

# Strix itself. A binary on PATH by default; an absolute path when it was
# installed somewhere launchd cannot see (a launchd agent inherits a minimal
# PATH, which is the usual reason a service that works in a shell fails as an
# agent).
BIN = os.getenv("STRIX_BIN", "strix")

# Where scan artifacts are kept. Each run gets its own subdirectory and Strix is
# executed with that as its working directory, so the run's output cannot be
# confused with any other run's.
RUNS_DIR = Path(os.getenv("STRIX_RUNS_DIR", "./strix_runs")).expanduser().resolve()

# Extra arguments appended to every Strix invocation, space-separated. An escape
# hatch for flags this service does not model — it is the operator's own machine
# and their own scanner, so guessing on their behalf is worse than letting them
# say.
EXTRA_ARGS = os.getenv("STRIX_EXTRA_ARGS", "").split()

# ─── The model Strix reasons with ───────────────────────────────────────────

# LiteLLM model id. The whole point of running here is that this can be a local
# model: "ollama_chat/qwen3-coder:30b" rather than "openai/gpt-4o". The
# ollama_chat prefix (not plain "ollama") is the one that supports tool calling,
# without which Strix cannot drive anything.
LLM = os.getenv("STRIX_LLM", "ollama_chat/qwen3-coder:30b")

# LiteLLM's LLM_API_BASE. On this host Ollama is on localhost, so it is reached
# directly and skips the public edge and its gateway entirely.
LLM_API_BASE = os.getenv("STRIX_LLM_API_BASE", "http://localhost:11434")

# Only needed for a hosted provider. Local Ollama needs no key.
LLM_API_KEY = os.getenv("STRIX_LLM_API_KEY", "")

# ─── Scope ──────────────────────────────────────────────────────────────────

# Comma-separated hosts, wildcards, CIDRs or URL prefixes that may be scanned.
#
# EMPTY MEANS DENY EVERYTHING. This is the one setting that must be wrong in the
# safe direction: an unconfigured scanner that scans nothing is a non-event,
# whereas one that scans anything reachable is an incident with someone else's
# name on it. See scope.py.
TARGET_ALLOWLIST = os.getenv("STRIX_TARGET_ALLOWLIST", "")

# Permit targets resolving into loopback, RFC1918 or link-local space without
# naming that range in the allowlist. Off by default: a public hostname that
# resolves inward is the shape of an SSRF pivot, not of a normal target.
ALLOW_PRIVATE_TARGETS = os.getenv("STRIX_ALLOW_PRIVATE_TARGETS", "").lower() in ("1", "true", "yes")

# ─── Capacity ───────────────────────────────────────────────────────────────

# Concurrent scans. One, by default: a scan is Docker containers plus a 30B
# model on the same GPU that serves the rest of the platform, so a second
# simultaneous scan is contention rather than throughput.
MAX_CONCURRENT = max(1, int(os.getenv("STRIX_MAX_CONCURRENT", "1")))

# How many runs may be waiting. Beyond this the service answers 503 with a
# Retry-After rather than accepting work it has no prospect of starting.
QUEUE_MAX = max(1, int(os.getenv("STRIX_QUEUE_MAX", "8")))

# Wall-clock ceiling on one scan. With a local model the money budget is
# effectively zero, which makes time the budget that actually bounds a run.
MAX_RUNTIME_SECONDS = int(os.getenv("STRIX_MAX_RUNTIME_SECONDS", "7200"))

# How long the process gets to exit after SIGTERM before it is killed outright.
TERM_GRACE_SECONDS = float(os.getenv("STRIX_TERM_GRACE_SECONDS", "20"))

# ─── Retention ──────────────────────────────────────────────────────────────

# Artifacts are deleted after this many days. Findings are already copied into
# Postgres by the caller, and Strix's artifacts can contain credentials it
# discovered — so keeping them indefinitely is a liability, not an archive.
RETENTION_DAYS = int(os.getenv("STRIX_RETENTION_DAYS", "14"))

# How much of a run's output is kept in memory for the API to return. Enough to
# diagnose a failure without turning the run registry into a log store.
LOG_TAIL_BYTES = int(os.getenv("STRIX_LOG_TAIL_BYTES", "8192"))

# ─── Auth ───────────────────────────────────────────────────────────────────

# Shared secret for the gateway JWT. Unset means the service authenticates
# nothing — see auth.py for why that is the documented local-development
# behaviour rather than an oversight.
JWT_SECRET = os.getenv("STRIX_JWT_SECRET", "")

# Slack allowed on the token's time claims, in seconds. The caller truncates
# `iat` down to a whole second, so a verifier a few milliseconds behind sees an
# `iat` in the future and rejects a perfectly good token.
JWT_LEEWAY = int(os.getenv("STRIX_JWT_LEEWAY", "60"))

# ─── Process ────────────────────────────────────────────────────────────────

HOST = os.getenv("STRIX_HOST", "0.0.0.0")

# 11436 sits one above the image service's 11435, which sits one above Ollama's
# 11434 — adjacent on purpose, so the three workstation services are obviously
# a set.
PORT = int(os.getenv("STRIX_PORT", "11436"))

LOG_LEVEL = os.getenv("STRIX_LOG_LEVEL", "info")
