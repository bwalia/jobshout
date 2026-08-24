from __future__ import annotations

import os
from pathlib import Path

import pytest

from app.config.settings import ConfigError, load_settings
from tests.conftest import make_git_repo


def test_load_settings_allows_missing_local_repo(monkeypatch, tmp_path):
    monkeypatch.setenv("GITHUB_TOKEN", "tok")
    monkeypatch.setenv("LOCAL_REPO_PATH", "")
    monkeypatch.setenv("OLLAMA_HOST", "https://ollama.example")
    monkeypatch.setenv("MODEL", "qwen3-coder:30b")
    settings = load_settings()
    assert settings.local_repo_path is None
    assert settings.github_token == "tok"
    assert settings.ollama_host == "https://ollama.example"


def test_load_settings_rejects_non_git_path(monkeypatch, tmp_path):
    monkeypatch.setenv("GITHUB_TOKEN", "tok")
    monkeypatch.setenv("LOCAL_REPO_PATH", str(tmp_path / "not-a-repo"))
    (tmp_path / "not-a-repo").mkdir()
    with pytest.raises(ConfigError, match="not a git repository"):
        load_settings()


def test_load_settings_resolves_git_path(monkeypatch, tmp_path):
    repo = make_git_repo(tmp_path / "clone")
    monkeypatch.setenv("GITHUB_TOKEN", "tok")
    monkeypatch.setenv("LOCAL_REPO_PATH", str(repo))
    settings = load_settings()
    assert settings.local_repo_path == Path(repo).resolve()
