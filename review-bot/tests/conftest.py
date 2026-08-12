from __future__ import annotations

from pathlib import Path

import pytest
from git import Repo

from app.config.settings import Settings


def make_git_repo(path: Path, origin_url: str | None = "https://github.com/acme/widgets.git") -> Path:
    """An initialized git repo with one commit and (optionally) an origin remote."""
    path.mkdir(parents=True, exist_ok=True)
    repo = Repo.init(path)
    (path / "README.md").write_text("fixture\n")
    repo.index.add(["README.md"])
    repo.index.commit("init")
    if origin_url:
        repo.create_remote("origin", origin_url)
    return path


@pytest.fixture
def local_repo(tmp_path: Path) -> Path:
    return make_git_repo(tmp_path / "local-repo")


@pytest.fixture
def settings(local_repo: Path) -> Settings:
    return Settings(
        github_token="test-token",
        local_repo_path=local_repo,
        ollama_host="http://localhost:11434",
        model="test-model",
        map_refresh_commits=30,
    )
