from __future__ import annotations

import threading
import time

import pytest
from mcp.server.mcpserver.exceptions import ToolError

import app.jobs as jobs
import app.mcp_server as srv
from app.jobs import JobRunner
from app.repos import RepoRegistry
from tests.test_jobs import StubGitHubClient, make_result, wait, wait_until


@pytest.fixture
def server_env(settings, monkeypatch):
    StubGitHubClient.created = []
    StubGitHubClient.bodies = []
    monkeypatch.setattr(jobs, "GitHubClient", StubGitHubClient)
    registry = RepoRegistry(settings)
    runner = JobRunner(settings, registry)
    monkeypatch.setattr(srv, "_runner", runner)
    monkeypatch.setattr(srv, "_registry", registry)
    return runner


def test_start_review_returns_job_immediately(server_env, monkeypatch):
    monkeypatch.setattr(jobs, "review_pull_request", lambda **kw: make_result(kw["pr_number"]))
    response = srv.start_review("acme/widgets", 1, dry_run=True, force=True)
    assert set(response) == {"job_id", "state", "queue_position"}

    wait(server_env.get(response["job_id"]))
    status = srv.review_status(response["job_id"])
    assert status["state"] == "done"
    assert status["result"]["decision"] == "FIX"


def test_disallowed_repo_is_clean_tool_error(server_env):
    with pytest.raises(ToolError, match="allowlist"):
        srv.start_review("evil/repo", 1)


def test_unknown_job_id_is_clean_tool_error(server_env):
    with pytest.raises(ToolError, match="not found"):
        srv.review_status("deadbeef")


def test_list_repos_returns_allowlist(server_env):
    assert srv.list_repos() == {"allowed": ["acme/widgets"]}


def test_status_stays_fast_while_review_runs(server_env, monkeypatch):
    gate = threading.Event()

    def slow_review(settings, client, pr_number, dry_run, on_progress):
        gate.wait(5)
        return make_result(pr_number)

    monkeypatch.setattr(jobs, "review_pull_request", slow_review)
    first = srv.start_review("acme/widgets", 1, dry_run=True, force=True)
    wait_until(lambda: srv.review_status(first["job_id"])["state"] == "running")

    started = time.monotonic()
    second = srv.start_review("acme/widgets", 2, dry_run=True, force=True)
    status = srv.review_status(second["job_id"])
    elapsed = time.monotonic() - started

    assert elapsed < 1.0  # tools must never block on the running review
    assert second["queue_position"] == 1
    assert status["state"] == "queued"
    gate.set()
    wait(server_env.get(first["job_id"]))
    wait(server_env.get(second["job_id"]))


def test_prime_uses_same_job_api(server_env, monkeypatch):
    class FakeMap:
        built_sha = "a" * 40
        text = "map"

    monkeypatch.setattr(jobs, "load_cached", lambda s: None)
    monkeypatch.setattr(jobs, "build_map", lambda s, on_progress: FakeMap())
    response = srv.prime("acme/widgets")
    job = wait(server_env.get(response["job_id"]))
    assert job.result["primed"] is True
