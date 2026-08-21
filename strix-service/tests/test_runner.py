"""Runner tests — findings parsing, exit-code classification, and execution."""

import asyncio
import json

import pytest

from app import runner as runner_module
from app.runner import Busy, parse_findings
from app.store import BUDGET_EXCEEDED, COMPLETED, FAILED


# ─── findings parsing ───────────────────────────────────────────────────────

def write_vulns(run_dir, vulns, nested="strix_runs/abc"):
    target = run_dir / nested
    target.mkdir(parents=True, exist_ok=True)
    (target / "vulnerabilities.json").write_text(json.dumps({"vulnerabilities": vulns}))


def test_parse_findings_reads_a_nested_report(tmp_path):
    write_vulns(tmp_path, [{"id": "v1", "title": "SQLi", "severity": "high", "cvss_score": 8.1}])
    findings = parse_findings(tmp_path)
    assert len(findings) == 1
    assert findings[0].title == "SQLi"
    assert findings[0].cvss_score == 8.1


def test_parse_findings_returns_empty_when_there_is_no_report(tmp_path):
    assert parse_findings(tmp_path) == []


def test_parse_findings_survives_malformed_json(tmp_path):
    d = tmp_path / "strix_runs" / "abc"
    d.mkdir(parents=True)
    (d / "vulnerabilities.json").write_text("{not json")
    # Degrades to "nothing parsed" rather than taking the run down with it.
    assert parse_findings(tmp_path) == []


def test_parse_findings_accepts_a_bare_list(tmp_path):
    d = tmp_path / "strix_runs" / "abc"
    d.mkdir(parents=True)
    (d / "vulnerabilities.json").write_text(json.dumps([{"title": "XSS", "severity": "medium"}]))
    assert parse_findings(tmp_path)[0].title == "XSS"


def test_untitled_finding_gets_a_placeholder(tmp_path):
    write_vulns(tmp_path, [{"severity": "low"}])
    assert parse_findings(tmp_path)[0].title == "(untitled finding)"


@pytest.mark.parametrize("raw,expected", [
    ("HIGH", "high"),
    ("Informational", "info"),
    ("moderate", "medium"),
    ("severe", "critical"),
    ("", "info"),
    ("nonsense", "info"),
])
def test_severity_is_normalized_to_what_the_database_accepts(tmp_path, raw, expected):
    # Migration 026 constrains severity to critical/high/medium/low/info. A value
    # outside that set would fail the caller's insert and take every other
    # finding in the same transaction with it.
    write_vulns(tmp_path, [{"title": "x", "severity": raw}])
    assert parse_findings(tmp_path)[0].severity == expected


def test_non_numeric_cvss_becomes_zero(tmp_path):
    write_vulns(tmp_path, [{"title": "x", "severity": "low", "cvss_score": "n/a"}])
    assert parse_findings(tmp_path)[0].cvss_score == 0.0


# ─── exit-code classification ───────────────────────────────────────────────

def test_exit_zero_is_a_clean_completed_scan(runner):
    assert runner._classify(0, "", [])[0] == COMPLETED


def test_exit_two_is_completed_not_failed(runner):
    # Strix uses 2 for "vulnerabilities found". Treating it as failure would
    # report every scan that actually found something as broken.
    assert runner._classify(2, "", [])[0] == COMPLETED


def test_budget_stop_is_its_own_status(runner):
    status, error = runner._classify(1, "LLM budget of $10 exceeded, stopping", [])
    assert status == BUDGET_EXCEEDED
    assert "budget" in error


def test_a_rate_limit_message_is_not_mistaken_for_a_budget_stop(runner):
    # The Go client tested for "budget" OR "exceeded", so this exact message was
    # mislabelled as a budget stop rather than a failure.
    status, _ = runner._classify(1, "rate limit exceeded", [])
    assert status == FAILED


def test_plain_failure_carries_the_output(runner):
    status, error = runner._classify(1, "docker: daemon not running", [])
    assert status == FAILED
    assert "daemon not running" in error


# ─── execution ──────────────────────────────────────────────────────────────

def make_run(store, target="https://example.com", **kw):
    return store.create(run_ref=kw.pop("run_ref", ""), target=target, scan_mode="quick",
                        max_budget=kw.pop("max_budget", None), instruction="",
                        requested_by="", addresses=["93.184.216.34"])


async def drain(runner):
    """Wait for every scheduled task, including any spawned while draining.

    The sleep(0) is load-bearing. Awaiting gather() over tasks that have already
    finished returns without giving the event loop a turn, so the done-callbacks
    that remove them from runner._tasks never run — and the loop below spins
    forever on a set of finished tasks. Yielding once lets those callbacks fire.
    """
    while True:
        tasks = list(runner._tasks)
        if not tasks:
            return
        await asyncio.gather(*tasks, return_exceptions=True)
        await asyncio.sleep(0)


async def test_a_successful_scan_records_its_findings(runner, store, fake_strix, monkeypatch):
    monkeypatch.setattr(
        runner_module.config, "BIN",
        fake_strix(exit_code=2, findings=[{"id": "v1", "title": "SQLi", "severity": "critical"}]),
    )
    run = make_run(store)
    runner.submit(run)
    await drain(runner)

    assert run.status == COMPLETED
    assert run.finding_count == 1
    assert run.findings[0].severity == "critical"
    assert run.completed_at is not None


async def test_a_missing_binary_fails_the_run_with_a_useful_message(runner, store, monkeypatch):
    monkeypatch.setattr(runner_module.config, "BIN", "/nonexistent/strix")
    run = make_run(store)
    runner.submit(run)
    await drain(runner)

    assert run.status == FAILED
    assert "not found" in run.error
    # The launchd PATH trap is the most common cause, so it is named in the error
    # rather than left for someone to rediscover at 3am.
    assert "PATH" in run.error


async def test_a_scan_that_overruns_is_terminated(runner, store, fake_strix, monkeypatch):
    monkeypatch.setattr(runner_module.config, "BIN", fake_strix(exit_code=0, sleep=30))
    runner.max_runtime = 1
    run = make_run(store)
    runner.submit(run)
    await drain(runner)

    assert run.status == FAILED
    assert "runtime limit" in run.error


async def test_output_is_captured_to_the_log_and_the_tail(runner, store, fake_strix, monkeypatch):
    monkeypatch.setattr(runner_module.config, "BIN", fake_strix(exit_code=0, output="scanning target"))
    run = make_run(store)
    runner.submit(run)
    await drain(runner)

    assert "scanning target" in run.log_tail
    assert (store.run_dir(run.run_id) / "strix.log").exists()


async def test_the_queue_refuses_work_beyond_its_limit(runner, store, fake_strix, monkeypatch):
    monkeypatch.setattr(runner_module.config, "BIN", fake_strix(exit_code=0, sleep=5))
    runs = [make_run(store) for _ in range(runner.queue_max)]
    for run in runs:
        runner.submit(run)

    with pytest.raises(Busy):
        runner.submit(make_run(store))

    for run in runs:
        await runner.cancel(run)
    await drain(runner)


async def test_budget_is_passed_through_to_the_scanner(runner, store):
    run = make_run(store, max_budget=25)
    assert "--max-budget" in runner.build_args(run)
    assert "25" in runner.build_args(run)


async def test_no_budget_flag_when_none_was_asked_for(runner, store):
    assert "--max-budget" not in runner.build_args(make_run(store))


def test_the_local_model_endpoint_is_passed_to_strix(runner, monkeypatch):
    monkeypatch.setattr(runner_module.config, "LLM", "ollama_chat/qwen3-coder:30b")
    monkeypatch.setattr(runner_module.config, "LLM_API_BASE", "http://localhost:11434")
    env = runner.build_env()
    assert env["STRIX_LLM"] == "ollama_chat/qwen3-coder:30b"
    assert env["LLM_API_BASE"] == "http://localhost:11434"


def test_no_api_base_is_set_for_a_hosted_provider(runner, monkeypatch):
    # An empty LLM_API_BASE would point LiteLLM at nothing rather than at the
    # provider's own default endpoint.
    monkeypatch.setattr(runner_module.config, "LLM_API_BASE", "")
    assert "LLM_API_BASE" not in runner.build_env()


async def test_cancelling_a_running_scan_marks_it_cancelled(runner, store, fake_strix, monkeypatch):
    monkeypatch.setattr(runner_module.config, "BIN", fake_strix(exit_code=0, sleep=30))
    run = make_run(store)
    runner.submit(run)
    for _ in range(100):
        await asyncio.sleep(0.05)
        if run.run_id in runner._processes:
            break
    assert await runner.cancel(run) is True
    await drain(runner)
    assert run.status == "cancelled"
