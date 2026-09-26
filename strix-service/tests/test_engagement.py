"""Engagement prompt and hollow-run detection."""

from app.engagement import (
    STANDING_ENGAGEMENT,
    compose_instruction,
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
