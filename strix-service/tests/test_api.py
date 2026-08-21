"""API tests — the contract the Go client will be written against."""

import time

import jwt
import pytest
from fastapi.testclient import TestClient

from app import config, main, scope
from app import store as store_module
from app.runner import Runner

SECRET = "api-test-secret-at-least-32-bytes-long!!"


@pytest.fixture
def client(tmp_path, monkeypatch):
    """An app whose store and runner are scoped to this test."""
    store = store_module.Store(tmp_path / "runs")
    monkeypatch.setattr(main, "store", store)
    monkeypatch.setattr(main, "runner", Runner(store, max_concurrent=1, queue_max=4, max_runtime=5))
    monkeypatch.setattr(scope, "RULES", scope.parse_rules("*.example.com"))
    monkeypatch.setattr(scope, "_resolve", lambda host: ("93.184.216.34",))
    monkeypatch.setattr(config, "JWT_SECRET", "")
    # No real subprocess: these tests are about the HTTP contract.
    monkeypatch.setattr(main.runner, "submit", lambda run: 0)
    with TestClient(main.app) as c:
        yield c


def test_health_needs_no_credentials(client):
    resp = client.get("/health")
    assert resp.status_code == 200
    assert resp.json()["status"] == "ok"


def test_root_reports_whether_anything_is_in_scope(client):
    body = client.get("/").json()
    assert body["backend"] == "strix"
    assert body["targets_in_scope"] == 1


def test_starting_a_scan_returns_a_run_id_immediately(client):
    resp = client.post("/api/scan", json={"target": "https://api.example.com", "scan_mode": "quick"})
    assert resp.status_code == 202
    body = resp.json()
    assert body["run_id"]
    assert body["status"] == "queued"


def test_an_out_of_scope_target_is_refused_with_403(client):
    resp = client.post("/api/scan", json={"target": "https://victim.invalid"})
    assert resp.status_code == 403
    assert "not in scope" in resp.json()["detail"]


def test_an_unknown_scan_mode_is_rejected(client):
    resp = client.post("/api/scan", json={"target": "https://api.example.com", "scan_mode": "nuclear"})
    assert resp.status_code == 400


def test_the_same_run_ref_never_starts_a_second_scan(client):
    first = client.post("/api/scan", json={"target": "https://api.example.com", "run_ref": "run-1"})
    second = client.post("/api/scan", json={"target": "https://api.example.com", "run_ref": "run-1"})

    assert first.status_code == 202
    # 200 rather than 202: nothing new was accepted.
    assert second.status_code == 200
    assert second.json()["existing"] is True
    assert second.json()["run_id"] == first.json()["run_id"]


def test_status_can_be_polled_by_run_id(client):
    run_id = client.post("/api/scan", json={"target": "https://api.example.com"}).json()["run_id"]
    resp = client.get(f"/api/scan/{run_id}")
    assert resp.status_code == 200
    assert resp.json()["target"] == "https://api.example.com"


def test_polling_an_unknown_run_is_404(client):
    assert client.get("/api/scan/does-not-exist").status_code == 404


def test_listing_scans_omits_findings(client):
    client.post("/api/scan", json={"target": "https://api.example.com"})
    body = client.get("/api/scans").json()
    assert body["count"] == 1
    assert "findings" not in body["runs"][0]


def test_cancelling_an_unknown_run_is_404(client):
    assert client.delete("/api/scan/does-not-exist").status_code == 404


def test_a_queue_that_is_full_answers_503_with_retry_after(client, monkeypatch):
    from app.runner import Busy

    def full(run):
        raise Busy("queue is full")
    monkeypatch.setattr(main.runner, "submit", full)

    resp = client.post("/api/scan", json={"target": "https://api.example.com"})
    assert resp.status_code == 503
    assert resp.headers["Retry-After"] == "60"


# ─── auth over the wire ─────────────────────────────────────────────────────

@pytest.fixture
def secured(client, monkeypatch):
    monkeypatch.setattr(config, "JWT_SECRET", SECRET)
    return client


def test_health_stays_open_when_auth_is_on(secured):
    assert secured.get("/health").status_code == 200


def test_scanning_without_a_token_is_401(secured):
    resp = secured.post("/api/scan", json={"target": "https://api.example.com"})
    assert resp.status_code == 401


def test_scanning_with_a_valid_token_works(secured):
    now = int(time.time())
    token = jwt.encode({"app": "jobshout", "iat": now, "exp": now + 600}, SECRET, algorithm="HS256")
    resp = secured.post("/api/scan", json={"target": "https://api.example.com"},
                        headers={"x-api-key": token})
    assert resp.status_code == 202


def test_an_empty_allowlist_refuses_even_an_authenticated_caller(secured, monkeypatch):
    # Auth widens who may ask; it never widens what may be scanned.
    monkeypatch.setattr(scope, "RULES", [])
    now = int(time.time())
    token = jwt.encode({"app": "jobshout", "iat": now, "exp": now + 600}, SECRET, algorithm="HS256")
    resp = secured.post("/api/scan", json={"target": "https://api.example.com"},
                        headers={"x-api-key": token})
    assert resp.status_code == 403
    assert "no targets are in scope" in resp.json()["detail"]
