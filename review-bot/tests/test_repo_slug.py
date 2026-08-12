from __future__ import annotations

import pytest

from app.github.client import GitHubError, repo_slug
from tests.conftest import make_git_repo


def test_https_url(tmp_path):
    path = make_git_repo(tmp_path / "r1", "https://github.com/acme/widgets.git")
    assert repo_slug(path) == "acme/widgets"


def test_ssh_url(tmp_path):
    path = make_git_repo(tmp_path / "r2", "git@github.com:acme/widgets.git")
    assert repo_slug(path) == "acme/widgets"


def test_url_without_dot_git(tmp_path):
    path = make_git_repo(tmp_path / "r3", "https://github.com/acme/widgets")
    assert repo_slug(path) == "acme/widgets"


def test_no_origin_remote(tmp_path):
    path = make_git_repo(tmp_path / "r4", origin_url=None)
    with pytest.raises(GitHubError):
        repo_slug(path)
