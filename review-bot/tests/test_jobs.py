from __future__ import annotations

import threading
import time

import pytest

import app.jobs as jobs
from app.github.client import GitHubError, PullRequest
from app.github.markers import marker, started_comment
from app.jobs import JobRunner, review_to_dict
from app.repos import RepoNotAllowed, RepoRegistry
from app.reviewer.agent import NothingToReview, ReviewResult
from app.reviewer.schema import Issue, Review

HEAD_SHA = "cafebabe" + "0" * 32


class StubGitHubClient:
    """Records comment posts; PR head is fixed. `bodies` feeds already_started."""

    created: list["StubGitHubClient"] = []
    bodies: list[str] = []

    def __init__(self, token: str, slug: str) -> None:
        self.token = token
        self.slug = slug
        self.posted: list[str] = []
        StubGitHubClient.created.append(self)

    def get_pull_request(self, number: int) -> PullRequest:
        return PullRequest(number=number, title="a title", body="", head_sha=HEAD_SHA)

    def issue_comment_bodies(self, number: int) -> list[str]:
        return list(StubGitHubClient.bodies)

    def post_issue_comment(self, number: int, body: str) -> str:
        self.posted.append(body)
        return "https://example.test/comment"


def make_result(pr_number: int, url: str | None = "https://example.test/review") -> ReviewResult:
    issue = Issue(
        severity="high", category="breaks", file="a.py", line=3,
        title="broken", reason="because", suggestion="fix it",
    )
    return ReviewResult(
        pull_request=PullRequest(number=pr_number, title="a title", body="", head_sha=HEAD_SHA),
        review=Review(verdict="v", summary="s", inline=[issue], orphaned=[]),
        url=url,
    )


def wait(job, timeout: float = 5.0):
    deadline = time.time() + timeout
    while time.time() < deadline:
        if job.state in ("done", "failed"):
            return job
        time.sleep(0.01)
    raise AssertionError(f"job {job.id} did not finish (state={job.state})")


def wait_until(predicate, timeout: float = 5.0) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        if predicate():
            return
        time.sleep(0.01)
    raise AssertionError("condition not reached in time")


@pytest.fixture
def runner(settings, monkeypatch):
    StubGitHubClient.created = []
    StubGitHubClient.bodies = []
    monkeypatch.setattr(jobs, "GitHubClient", StubGitHubClient)
    return JobRunner(settings, RepoRegistry(settings))


def test_two_jobs_run_strictly_serially(runner, monkeypatch):
    active = 0
    max_active = 0
    lock = threading.Lock()

    def fake_review(settings, client, pr_number, dry_run, on_progress):
        nonlocal active, max_active
        with lock:
            active += 1
            max_active = max(max_active, active)
        time.sleep(0.15)
        with lock:
            active -= 1
        return make_result(pr_number)

    monkeypatch.setattr(jobs, "review_pull_request", fake_review)
    first = runner.submit_review("acme/widgets", 1, dry_run=True, force=True)
    second = runner.submit_review("acme/widgets", 2, dry_run=True, force=True)
    wait(first)
    wait(second)
    assert max_active == 1
    assert first.state == second.state == "done"


def test_states_transition_and_queue_position(runner, monkeypatch):
    gate = threading.Event()

    def fake_review(settings, client, pr_number, dry_run, on_progress):
        gate.wait(5)
        return make_result(pr_number)

    monkeypatch.setattr(jobs, "review_pull_request", fake_review)
    first = runner.submit_review("acme/widgets", 1, dry_run=True, force=True)
    second = runner.submit_review("acme/widgets", 2, dry_run=True, force=True)

    wait_until(lambda: first.state == "running")
    assert second.state == "queued"
    assert runner.snapshot(second.id)["queue_position"] == 1
    assert "queue_position" not in runner.snapshot(first.id)

    gate.set()
    assert wait(first).state == "done"
    assert wait(second).state == "done"
    assert "result" in runner.snapshot(first.id)


def test_failure_captures_error(runner, monkeypatch):
    def fake_review(settings, client, pr_number, dry_run, on_progress):
        raise GitHubError("boom")

    monkeypatch.setattr(jobs, "review_pull_request", fake_review)
    job = wait(runner.submit_review("acme/widgets", 1, dry_run=True, force=True))
    assert job.state == "failed"
    assert "GitHubError" in job.error and "boom" in job.error


def test_nothing_to_review_is_done_not_failed(runner, monkeypatch):
    def fake_review(settings, client, pr_number, dry_run, on_progress):
        raise NothingToReview("no code changes")

    monkeypatch.setattr(jobs, "review_pull_request", fake_review)
    job = wait(runner.submit_review("acme/widgets", 1, dry_run=True, force=True))
    assert job.state == "done"
    assert job.result["nothing_to_review"] is True


def test_dedup_skips_when_marker_present(runner, monkeypatch):
    StubGitHubClient.bodies = [started_comment(HEAD_SHA)]
    calls = []
    monkeypatch.setattr(jobs, "review_pull_request", lambda **kw: calls.append(1) or make_result(1))

    job = wait(runner.submit_review("acme/widgets", 1, dry_run=False, force=False))
    assert job.state == "done"
    assert job.result["already_started"] is True
    assert calls == []
    assert StubGitHubClient.created[-1].posted == []  # no duplicate marker comment


def test_force_bypasses_dedup(runner, monkeypatch):
    StubGitHubClient.bodies = [started_comment(HEAD_SHA)]

    def fake_review(settings, client, pr_number, dry_run, on_progress):
        return make_result(pr_number)

    monkeypatch.setattr(jobs, "review_pull_request", fake_review)
    job = wait(runner.submit_review("acme/widgets", 1, dry_run=False, force=True))
    assert job.state == "done"
    assert job.result["decision"] == "FIX"


def test_dry_run_posts_no_marker(runner, monkeypatch):
    def fake_review(settings, client, pr_number, dry_run, on_progress):
        return make_result(pr_number, url=None)

    monkeypatch.setattr(jobs, "review_pull_request", fake_review)
    job = wait(runner.submit_review("acme/widgets", 1, dry_run=True, force=True))
    assert job.state == "done"
    assert StubGitHubClient.created[-1].posted == []


def test_real_run_posts_marker_ack(runner, monkeypatch):
    def fake_review(settings, client, pr_number, dry_run, on_progress):
        return make_result(pr_number)

    monkeypatch.setattr(jobs, "review_pull_request", fake_review)
    job = wait(runner.submit_review("acme/widgets", 1, dry_run=False, force=False))
    assert job.state == "done"
    posted = StubGitHubClient.created[-1].posted
    assert len(posted) == 1 and marker(HEAD_SHA) in posted[0]


def test_submit_rejects_disallowed_repo(runner):
    with pytest.raises(RepoNotAllowed):
        runner.submit_review("evil/repo", 1)


def test_unknown_job_id_snapshot_is_none(runner):
    assert runner.snapshot("nope") is None


def test_run_ref_returns_the_same_job(runner, monkeypatch):
    monkeypatch.setattr(jobs, "review_pull_request", lambda **kw: make_result(kw["pr_number"]))
    first = runner.submit_review("acme/widgets", 1, dry_run=True, force=True, run_ref="run-1")
    second = runner.submit_review("acme/widgets", 1, dry_run=True, force=True, run_ref="run-1")
    assert first.id == second.id
    wait(first)


def test_timeout_fails_job_and_sticks(settings, monkeypatch):
    StubGitHubClient.created = []
    StubGitHubClient.bodies = []
    monkeypatch.setattr(jobs, "GitHubClient", StubGitHubClient)

    def slow_review(settings, client, pr_number, dry_run, on_progress):
        time.sleep(0.6)
        return make_result(pr_number)

    monkeypatch.setattr(jobs, "review_pull_request", slow_review)
    runner = JobRunner(settings, RepoRegistry(settings), timeout=0.1)
    job = wait(runner.submit_review("acme/widgets", 1, dry_run=True, force=True))
    assert job.state == "failed"
    assert "Timed out" in job.error
    time.sleep(0.7)  # the orphaned body finishing later must not overwrite the failure
    assert job.state == "failed"


def test_prime_job_builds_and_caches(runner, monkeypatch):
    built = []

    class FakeMap:
        built_sha = HEAD_SHA
        text = "map text"

    monkeypatch.setattr(jobs, "load_cached", lambda s: None)
    monkeypatch.setattr(jobs, "build_map", lambda s, on_progress: built.append(1) or FakeMap())
    job = wait(runner.submit_prime("acme/widgets"))
    assert job.state == "done"
    assert job.result == {"primed": True, "built_sha": HEAD_SHA, "chars": len("map text")}

    monkeypatch.setattr(jobs, "load_cached", lambda s: FakeMap())
    job = wait(runner.submit_prime("acme/widgets"))
    assert job.result["cached"] is True
    assert built == [1]  # second prime did not rebuild


def test_review_to_dict_keeps_computed_properties():
    data = review_to_dict(make_result(7))
    assert data["decision"] == "FIX"
    assert data["blocking"][0]["title"] == "broken"
    assert data["inline"][0]["is_critical"] is True
    assert data["url"] == "https://example.test/review"
    assert data["pr_number"] == 7

    merge = ReviewResult(
        pull_request=PullRequest(number=8, title="t", body="", head_sha=HEAD_SHA),
        review=Review(verdict="v", summary="s", inline=[], orphaned=[]),
        url=None,
    )
    data = review_to_dict(merge)
    assert data["decision"] == "MERGE"
    assert data["blocking"] == []
