from __future__ import annotations

from app.github.markers import already_started, marker, started_comment


class StubClient:
    def __init__(self, bodies: list[str]) -> None:
        self._bodies = bodies

    def issue_comment_bodies(self, number: int) -> list[str]:
        return self._bodies


def test_marker_round_trip():
    sha = "abc123def456"
    assert marker(sha) in started_comment(sha)


def test_marker_is_sha_specific():
    assert marker("aaa") not in started_comment("bbb")


def test_already_started_finds_marker_in_comments():
    client = StubClient(["hello", started_comment("abc123")])
    assert already_started(client, 1, "abc123")


def test_already_started_false_for_other_sha():
    client = StubClient([started_comment("abc123")])
    assert not already_started(client, 1, "fff999")


def test_already_started_false_with_no_comments():
    assert not already_started(StubClient([]), 1, "abc123")
