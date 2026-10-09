"""Engagement prompt and hollow-run detection."""

import pytest

from app.engagement import (
    STANDING_ENGAGEMENT,
    compose_instruction,
    fabricated_domain,
    host_of,
    read_report_markdown,
    target_engaged,
)


def test_host_of_parses_https():
    assert host_of("https://int.jobshout.co.uk/path") == "int.jobshout.co.uk"


def test_compose_includes_standing_and_operator_note():
    text = compose_instruction("https://int.jobshout.co.uk", "Focus on /api")
    assert STANDING_ENGAGEMENT in text
    assert "https://int.jobshout.co.uk" in text
    assert "Focus on /api" in text


def test_target_engaged_requires_http_hint(tmp_path):
    (tmp_path / "strix.log").write_text(
        "strix -n --target https://int.jobshout.co.uk --scan-mode quick\n"
        "listing sandbox /workspace\n"
    )
    assert target_engaged(tmp_path, "https://int.jobshout.co.uk") is False


def test_target_engaged_true_when_http_to_host(tmp_path):
    (tmp_path / "strix.log").write_text(
        "curl -sI https://int.jobshout.co.uk/\n"
        "HTTP/1.1 200 OK\n"
    )
    assert target_engaged(tmp_path, "https://int.jobshout.co.uk") is True


def test_read_report_markdown(tmp_path):
    nested = tmp_path / "strix_runs" / "abc"
    nested.mkdir(parents=True)
    (nested / "penetration_test_report.md").write_text("# Report\nNo confirmed issues.\n")
    assert "No confirmed issues" in read_report_markdown(tmp_path)


@pytest.mark.parametrize("text,expected", [
    ("the acme.example portal leaked creds", "acme.example"),
    ("see http://example.com/login", "example.com"),
    ("visit www.example.org for docs", "www.example.org"),
    ("probed host.invalid and host.test", "host.invalid"),
])
def test_fabricated_domain_flags_reserved_hosts(text, expected):
    assert fabricated_domain(text) == expected


@pytest.mark.parametrize("text", [
    "",
    "tested https://int.jobshout.co.uk/ — looks fine",
    "for example, the login form is weak",   # 'example' as a word, no reserved host
    "the service runs at foo.test.realco.com",  # '.test' is an internal label, not the TLD
    "notexample.com is a real registered domain",  # 'example.com' inside a larger label
])
def test_fabricated_domain_passes_real_reports(text):
    assert fabricated_domain(text) == ""


def test_fabricated_domain_skipped_when_target_is_itself_reserved():
    # The test suite literally scans *.example.com; a report about that target
    # is legitimate, not a hallucination.
    assert fabricated_domain("scanned api.example.com thoroughly", "api.example.com") == ""
