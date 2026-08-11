"""Configuration loading and validation."""

from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
from dataclasses import dataclass
from pathlib import Path

from dotenv import load_dotenv


class ConfigError(RuntimeError):
    """Raised when configuration is missing or invalid."""


# How many commits HEAD may advance past the cached repo map before we rebuild it.
# A map that has drifted far from the code misdescribes it, which is worse than
# none; too low and we pay the one-time build cost constantly. Tunable via env.
DEFAULT_MAP_REFRESH_COMMITS = 30


def _github_token() -> str:
    """GITHUB_TOKEN if set, else borrow the gh CLI's token."""
    token = os.getenv("GITHUB_TOKEN", "").strip()
    if token:
        return token

    if not shutil.which("gh"):
        raise ConfigError("GITHUB_TOKEN is not set and the gh CLI is not installed.")

    result = subprocess.run(["gh", "auth", "token"], capture_output=True, text=True, timeout=15)
    token = result.stdout.strip()
    if result.returncode != 0 or not token:
        raise ConfigError("GITHUB_TOKEN is not set and `gh auth token` failed. Run: gh auth login")
    return token


@dataclass(frozen=True)
class Settings:
    github_token: str
    local_repo_path: Path
    ollama_host: str
    model: str
    map_refresh_commits: int

    @property
    def map_cache_dir(self) -> Path:
        """Where the cached repo map lives — keyed by repo path, outside the repo.

        Kept out of the target repo on purpose, same principle as the worktree:
        we never write into the code we are reviewing.
        """
        key = hashlib.sha256(str(self.local_repo_path).encode()).hexdigest()[:16]
        return Path.home() / ".cache" / "review-bot" / key


def load_settings() -> Settings:
    load_dotenv()

    raw_path = os.getenv("LOCAL_REPO_PATH", "").strip()
    if not raw_path:
        raise ConfigError("LOCAL_REPO_PATH is not set.")

    repo_path = Path(raw_path).expanduser().resolve()
    if not repo_path.is_dir():
        raise ConfigError(f"LOCAL_REPO_PATH does not exist: {repo_path}")
    if not (repo_path / ".git").exists():
        raise ConfigError(f"LOCAL_REPO_PATH is not a git repository: {repo_path}")

    try:
        refresh = int(os.getenv("MAP_REFRESH_COMMITS", str(DEFAULT_MAP_REFRESH_COMMITS)))
    except ValueError:
        refresh = DEFAULT_MAP_REFRESH_COMMITS

    return Settings(
        github_token=_github_token(),
        local_repo_path=repo_path,
        ollama_host=os.getenv("OLLAMA_HOST", "http://localhost:11434").strip().rstrip("/"),
        model=os.getenv("MODEL", "qwen3-coder:30b").strip(),
        map_refresh_commits=max(0, refresh),
    )
