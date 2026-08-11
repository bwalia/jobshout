"""Build, cache, and refresh a compact map of the repository.

The map is distilled repo knowledge — architecture, conventions, gotchas — that a
reviewer would otherwise rediscover on every PR. It is built once (expensive), cached
outside the repo, injected into each review, and rebuilt only when the code has drifted
far from it. This trades a one-time exploration for cheaper subsequent reviews.

Note the honest limit: the map replaces *repo-orientation*, not diff reading. The
changed files still get read fresh every review — that is the point of the PR.
"""

from __future__ import annotations

import json
import re
from dataclasses import dataclass
from pathlib import Path

from git import GitCommandError, Repo

from app.config.settings import Settings
from app.opencode.runner import run_agent
from app.prompts.prime import PRIME_PROMPT
from app.reviewer.workspace import head_worktree

_ANSI_RE = re.compile(r"\x1b\[[0-9;?]*[a-zA-Z]")
# OpenCode prints tool-call and status lines to stdout as it works; drop them.
_NOISE_PREFIXES = ("→", "> ", "|")
# The note begins at its first section heading (the prime prompt starts with "## Purpose").
_HEADING_RE = re.compile(r"^#{1,3}\s+\S")

PRIME_TIMEOUT = 1200  # a deep one-time exploration; allow more than a review


@dataclass(frozen=True)
class CachedMap:
    text: str
    built_sha: str


def _extract_markdown(stdout: str) -> str:
    """Recover the markdown note from OpenCode's stdout."""
    clean = _ANSI_RE.sub("", stdout)
    lines = clean.splitlines()

    # Prefer everything from the first real markdown heading onward.
    for index, line in enumerate(lines):
        if _HEADING_RE.match(line.strip()):
            return "\n".join(lines[index:]).strip()

    # Fallback: strip the obvious tool-call/status noise and keep the rest.
    kept = [ln for ln in lines if not ln.strip().startswith(_NOISE_PREFIXES)]
    return "\n".join(kept).strip()


def _map_file(settings: Settings) -> Path:
    return settings.map_cache_dir / "repo-map.md"


def _meta_file(settings: Settings) -> Path:
    return settings.map_cache_dir / "meta.json"


def load_cached(settings: Settings) -> CachedMap | None:
    map_file, meta_file = _map_file(settings), _meta_file(settings)
    if not (map_file.exists() and meta_file.exists()):
        return None
    try:
        sha = json.loads(meta_file.read_text()).get("built_sha", "")
        text = map_file.read_text()
    except (OSError, json.JSONDecodeError):
        return None
    if not (sha and text.strip()):
        return None
    return CachedMap(text=text, built_sha=sha)


def _save(settings: Settings, text: str, built_sha: str) -> None:
    settings.map_cache_dir.mkdir(parents=True, exist_ok=True)
    _map_file(settings).write_text(text)
    _meta_file(settings).write_text(json.dumps({"built_sha": built_sha}))


def _commits_since(repo: Repo, built_sha: str, head_sha: str) -> int | None:
    """How many commits HEAD is ahead of the map's build point. None if unknowable."""
    if built_sha == head_sha:
        return 0
    try:
        out = repo.git.rev_list("--count", f"{built_sha}..{head_sha}")
        return int(out.strip())
    except (GitCommandError, ValueError):
        return None  # built_sha not in history (rebased/force-pushed) -> rebuild


def is_stale(settings: Settings, cached: CachedMap, head_sha: str) -> bool:
    repo = Repo(settings.local_repo_path)
    drift = _commits_since(repo, cached.built_sha, head_sha)
    if drift is None:
        return True
    return drift > settings.map_refresh_commits


def build_map(settings: Settings, on_progress=lambda m: None) -> CachedMap:
    """Explore the repo once and cache a fresh map. Always rebuilds."""
    with head_worktree(settings.local_repo_path) as (workdir, head_sha):
        on_progress(f"Building repository map at {head_sha[:8]} (one-time, a few minutes)")
        stdout = run_agent(settings, workdir, PRIME_PROMPT, timeout=PRIME_TIMEOUT)

    text = _extract_markdown(stdout)
    if not text:
        # Do not cache an empty map; better to review without one than with nothing.
        raise RuntimeError("Repo map came back empty from OpenCode.")
    _save(settings, text, head_sha)
    return CachedMap(text=text, built_sha=head_sha)


def map_for_review(settings: Settings, on_progress=lambda m: None) -> CachedMap | None:
    """Return the cached map for a review, or None — never builds.

    The map is opt-in: `prime` builds it, reviews only consume it. A review must
    never silently trigger a multi-minute build, so if no map is cached we proceed
    without one, and if the cached map has gone stale we skip it (a misleading map
    is worse than none) and point the user at `prime --force`.
    """
    cached = load_cached(settings)
    if not cached:
        return None  # not primed — silent, this is the default

    head_sha = Repo(settings.local_repo_path).head.commit.hexsha
    if is_stale(settings, cached, head_sha):
        on_progress(
            f"Cached repo map is stale (built at {cached.built_sha[:8]}); skipping it. "
            "Run `prime --force` to refresh."
        )
        return None

    on_progress(f"Using cached repo map (built at {cached.built_sha[:8]})")
    return cached
