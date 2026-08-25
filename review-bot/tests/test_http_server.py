from __future__ import annotations

import threading

import pytest
from starlette.testclient import TestClient

import app.jobs as jobs
from app.http_server import create_app
from app.jobs import JobRunner
from app.repos import RepoRegistry
from tests.test_jobs import StubGitHubClient, make_result, wait


TOKEN = "test-review-bot-token"


@pytest.fixture
def client(settings, monkeypatch):
    StubGitHubClient.created = []
    StubGitHubClient.bodies = []
    monkeypatch.setattr(jobs, "GitHubClient", StubGitHubClient)
    monkeypatch.setattr(jobs, "review_pull_request", lambda **kw: make_result(kw["pr_number"]))
    registry = RepoRegistry(settings)
    runner = JobRunner(settings, registry)
    app = create_app(runner, registry, settings, TOKEN)
    return TestClient(app), runner


def test_health_is_unauthenticated(client):
    http, _ = client
    response = http.get("/health")
    assert response.status_code == 200
    assert response.json() == {"status": "ok"}


def test_api_requires_bearer(client):
    http, _ = client
    assert http.get("/api/repos").status_code == 401
    assert http.get("/api/repos", headers={"Authorization": "Bearer wrong"}).status_code == 401


def test_omitted_dry_run_defaults_false(client):
    http, runner = client
    created = http.post(
        "/api/reviews",
        headers={"Authorization": f"Bearer {TOKEN}"},
        json={"repo": "acme/widgets", "pr_number": 11, "force": True},
    )
    assert created.status_code == 202
    job = runner.get(created.json()["job_id"])
    assert job.params["dry_run"] is False
    wait(job)


def test_capabilities(client):
    http, _ = client
    response = http.get("/api/capabilities", headers={"Authorization": f"Bearer {TOKEN}"})
    assert response.status_code == 200
    body = response.json()
    assert "opencode" in body
    assert body["clones_dir"]["writable"] is True
    assert body["allowed"] == ["acme/widgets"]
    assert body["ollama"]["model"] == "test-model"


def test_list_repos(client):
    http, _ = client
    response = http.get("/api/repos", headers={"Authorization": f"Bearer {TOKEN}"})
    assert response.status_code == 200
    assert response.json() == {"allowed": ["acme/widgets"]}


def test_start_and_poll(client):
    http, runner = client
    created = http.post(
        "/api/reviews",
        headers={"Authorization": f"Bearer {TOKEN}"},
        json={"repo": "acme/widgets", "pr_number": 7, "dry_run": True, "force": True},
    )
    assert created.status_code == 202
    body = created.json()
    assert body["existing"] is False
    job_id = body["job_id"]
    wait(runner.get(job_id))
    status = http.get(f"/api/reviews/{job_id}", headers={"Authorization": f"Bearer {TOKEN}"})
    assert status.status_code == 200
    assert status.json()["state"] == "done"
    assert status.json()["result"]["decision"] == "FIX"


def test_unknown_job_is_404(client):
    http, _ = client
    response = http.get("/api/reviews/deadbeef", headers={"Authorization": f"Bearer {TOKEN}"})
    assert response.status_code == 404
    assert "restarted" in response.json()["error"]


def test_disallowed_repo_is_403(client):
    http, _ = client
    response = http.post(
        "/api/reviews",
        headers={"Authorization": f"Bearer {TOKEN}"},
        json={"repo": "evil/repo", "pr_number": 1},
    )
    assert response.status_code == 403


def test_run_ref_is_idempotent(client):
    http, runner = client
    payload = {
        "repo": "acme/widgets",
        "pr_number": 3,
        "dry_run": True,
        "force": True,
        "run_ref": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
    }
    first = http.post("/api/reviews", headers={"Authorization": f"Bearer {TOKEN}"}, json=payload)
    second = http.post("/api/reviews", headers={"Authorization": f"Bearer {TOKEN}"}, json=payload)
    assert first.status_code == 202
    assert second.status_code == 200
    assert second.json()["existing"] is True
    assert first.json()["job_id"] == second.json()["job_id"]
    wait(runner.get(first.json()["job_id"]))


def test_status_does_not_block_on_running_review(settings, monkeypatch):
    StubGitHubClient.created = []
    StubGitHubClient.bodies = []
    monkeypatch.setattr(jobs, "GitHubClient", StubGitHubClient)
    gate = threading.Event()

    def slow_review(**kw):
        gate.wait(5)
        return make_result(kw["pr_number"])

    monkeypatch.setattr(jobs, "review_pull_request", slow_review)
    registry = RepoRegistry(settings)
    runner = JobRunner(settings, registry)
    http = TestClient(create_app(runner, registry, settings, TOKEN))
    first = http.post(
        "/api/reviews",
        headers={"Authorization": f"Bearer {TOKEN}"},
        json={"repo": "acme/widgets", "pr_number": 1, "dry_run": True, "force": True},
    )
    second = http.post(
        "/api/reviews",
        headers={"Authorization": f"Bearer {TOKEN}"},
        json={"repo": "acme/widgets", "pr_number": 2, "dry_run": True, "force": True},
    )
    assert second.status_code == 202
    status = http.get(
        f"/api/reviews/{second.json()['job_id']}",
        headers={"Authorization": f"Bearer {TOKEN}"},
    )
    assert status.json()["state"] == "queued"
    gate.set()
    wait(runner.get(first.json()["job_id"]))
    wait(runner.get(second.json()["job_id"]))
