from __future__ import annotations

import json

import pytest
from git import Repo

from app.config.settings import ConfigError
from app.repos import DEFAULT_CLONES_DIR, RepoNotAllowed, RepoRegistry, github_clone_url
from app.reviewer.workspace import WorkspaceError
from tests.conftest import make_git_repo


def test_absent_config_allows_only_env_repo(settings, tmp_path):
    registry = RepoRegistry.load(settings, config_path=tmp_path / "missing.json")
    assert registry.allowed_slugs() == ["acme/widgets"]
    assert registry.clones_dir == DEFAULT_CLONES_DIR


def test_config_file_extends_allowlist(settings, tmp_path):
    config = tmp_path / "repos.json"
    config.write_text(json.dumps({"allowed": ["other/repo"], "clones_dir": str(tmp_path / "clones")}))
    registry = RepoRegistry.load(settings, config_path=config)
    assert registry.allowed_slugs() == ["acme/widgets", "other/repo"]
    assert registry.clones_dir == tmp_path / "clones"


def test_unknown_repo_rejected(settings):
    registry = RepoRegistry(settings)
    with pytest.raises(RepoNotAllowed):
        registry.require("evil/repo")


def test_malformed_slug_rejected(settings):
    registry = RepoRegistry(settings, allowed={"acme/widgets"})
    for bad in ["widgets", "a/b/c", "../etc", "acme/", "/widgets", "acme widgets"]:
        with pytest.raises(RepoNotAllowed):
            registry.require(bad)


def test_env_repo_resolves_to_existing_local_clone(settings):
    registry = RepoRegistry(settings)
    assert registry.ensure_clone("acme/widgets") == settings.local_repo_path


def test_settings_for_swaps_repo_path_only(settings, tmp_path):
    # Managed clone whose origin is a local path, so the freshening fetch is offline.
    upstream = make_git_repo(tmp_path / "upstream", origin_url=None)
    clones = tmp_path / "clones"
    clones.mkdir()
    Repo.clone_from(str(upstream), clones / "other__repo")

    registry = RepoRegistry(settings, allowed={"other/repo"}, clones_dir=clones)
    derived = registry.settings_for("other/repo")
    assert derived.local_repo_path == clones / "other__repo"
    assert derived.github_token == settings.github_token
    assert derived.model == settings.model
    assert derived.map_cache_dir != settings.map_cache_dir  # per-repo map cache


def test_settings_for_disallowed_repo_raises(settings):
    registry = RepoRegistry(settings)
    with pytest.raises(RepoNotAllowed):
        registry.settings_for("not/allowed")


def test_malformed_repos_json_is_a_config_error(settings, tmp_path):
    config = tmp_path / "repos.json"
    config.write_text("{not json")
    with pytest.raises(ConfigError, match="not valid JSON"):
        RepoRegistry.load(settings, config_path=config)


def test_half_written_clone_dir_is_a_clear_error(settings, tmp_path):
    clones = tmp_path / "clones"
    (clones / "other__repo").mkdir(parents=True)  # dir exists but is no git repo
    registry = RepoRegistry(settings, allowed={"other/repo"}, clones_dir=clones)
    with pytest.raises(WorkspaceError, match="delete it and retry"):
        registry.ensure_clone("other/repo")


def test_clone_dir_naming_uses_double_underscore(settings, tmp_path):
    upstream = make_git_repo(tmp_path / "upstream", origin_url=None)
    clones = tmp_path / "clones"
    clones.mkdir()
    Repo.clone_from(str(upstream), clones / "o__n")
    registry = RepoRegistry(settings, allowed={"o/n"}, clones_dir=clones)
    assert registry.ensure_clone("o/n") == clones / "o__n"


def test_github_clone_url_embeds_token():
    assert github_clone_url("acme/widgets") == "https://github.com/acme/widgets.git"
    assert github_clone_url("acme/widgets", "tok") == "https://x-access-token:tok@github.com/acme/widgets.git"


def test_allowlist_only_when_local_repo_path_is_none(tmp_path):
    from app.config.settings import Settings

    base = Settings(
        github_token="tok",
        local_repo_path=None,
        ollama_host="http://localhost:11434",
        model="m",
        map_refresh_commits=30,
    )
    config = tmp_path / "repos.json"
    config.write_text('{"allowed": ["acme/widgets"]}')
    registry = RepoRegistry.load(base, config_path=config)
    assert registry.allowed_slugs() == ["acme/widgets"]
    with pytest.raises(RepoNotAllowed):
        registry.require("other/repo")


def test_clones_dir_env_overrides_json(settings, tmp_path, monkeypatch):
    config = tmp_path / "repos.json"
    config.write_text(json.dumps({"allowed": ["other/repo"], "clones_dir": str(tmp_path / "from-json")}))
    monkeypatch.setenv("REVIEW_BOT_CLONES_DIR", str(tmp_path / "from-env"))
    registry = RepoRegistry.load(settings, config_path=config)
    assert registry.clones_dir == tmp_path / "from-env"
