"""Repo allowlist and managed clones for multi-repo reviews.

The registry is configured by an optional `repos.json` next to `.env`:

    {"allowed": ["owner/name", ...], "clones_dir": "~/.cache/review-bot/clones"}

When the file is absent the registry contains only the `.env` repo, so the
watcher keeps working with zero extra configuration. The `.env` repo is always
allowed and always resolves to the existing local clone; other repos are cloned
on demand under `clones_dir` and refreshed with a fetch on each use.
"""

from __future__ import annotations

import json
import re
from dataclasses import replace
from pathlib import Path

from git import GitCommandError, InvalidGitRepositoryError, NoSuchPathError, Repo

from app.config.settings import ConfigError, Settings
from app.github.client import GitHubError, repo_slug
from app.reviewer.workspace import WorkspaceError

DEFAULT_CLONES_DIR = Path.home() / ".cache" / "review-bot" / "clones"
CONFIG_FILENAME = "repos.json"

_SLUG_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9_.-]*/[A-Za-z0-9_.-]+$")


class RepoNotAllowed(RuntimeError):
    """Raised when a repo is malformed or not on the allowlist."""


class RepoRegistry:
    def __init__(self, base: Settings, allowed: set[str] | None = None, clones_dir: Path | None = None) -> None:
        self._base = base
        self.clones_dir = clones_dir or DEFAULT_CLONES_DIR
        try:
            self._base_slug: str | None = repo_slug(base.local_repo_path)
        except GitHubError:
            self._base_slug = None  # no origin remote; registry is config-only
        self.allowed: set[str] = set(allowed or ())
        if self._base_slug:
            self.allowed.add(self._base_slug)

    @classmethod
    def load(cls, base: Settings, config_path: Path | str = CONFIG_FILENAME) -> "RepoRegistry":
        """Build the registry from repos.json; absent file → only the .env repo."""
        path = Path(config_path)
        allowed: set[str] = set()
        clones_dir = DEFAULT_CLONES_DIR
        if path.is_file():
            try:
                data = json.loads(path.read_text())
            except json.JSONDecodeError as exc:
                # A typo here must not crash-loop the launchd service with a raw traceback.
                raise ConfigError(f"{path} is not valid JSON: {exc}") from exc
            allowed.update(data.get("allowed") or [])
            if data.get("clones_dir"):
                clones_dir = Path(data["clones_dir"]).expanduser()
        return cls(base=base, allowed=allowed, clones_dir=clones_dir)

    def allowed_slugs(self) -> list[str]:
        return sorted(self.allowed)

    def require(self, slug: str) -> str:
        """Validate slug format and allowlist membership; return the slug."""
        if not _SLUG_RE.match(slug):
            raise RepoNotAllowed(f"Not an 'owner/name' repo slug: {slug!r}")
        if slug not in self.allowed:
            raise RepoNotAllowed(
                f"Repo {slug!r} is not on the allowlist. Allowed: {', '.join(self.allowed_slugs()) or '(none)'}"
            )
        return slug

    def ensure_clone(self, slug: str) -> Path:
        """A ready-to-use local clone of the repo: fetched if cached, cloned if not."""
        self.require(slug)
        if slug == self._base_slug:
            return self._base.local_repo_path  # reuse the watcher's existing clone

        clone_path = self.clones_dir / slug.replace("/", "__")
        try:
            if clone_path.is_dir():
                Repo(clone_path).git.fetch("origin", "--prune")
            else:
                self.clones_dir.mkdir(parents=True, exist_ok=True)
                # HTTPS + the gh credential helper already authenticated on this machine.
                Repo.clone_from(f"https://github.com/{slug}.git", clone_path)
        except (InvalidGitRepositoryError, NoSuchPathError) as exc:
            # e.g. an interrupted first clone left a half-written directory behind.
            raise WorkspaceError(
                f"Clone at {clone_path} is not a usable git repo ({exc}); delete it and retry."
            ) from exc
        except GitCommandError as exc:
            raise WorkspaceError(f"Could not clone/fetch {slug}: {exc}") from exc
        return clone_path

    def settings_for(self, slug: str) -> Settings:
        """Per-repo Settings: same env, repo path swapped to this repo's clone.

        map_cache_dir is derived from local_repo_path, so each repo gets its own
        cached map automatically.
        """
        return replace(self._base, local_repo_path=self.ensure_clone(slug))
