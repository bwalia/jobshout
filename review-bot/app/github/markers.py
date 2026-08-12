"""Dedup markers for "review started" comments.

Shared by the watcher and the job runner: one review per head commit, survives
restarts with no local state. The marker is a hidden HTML comment keyed by the
PR's head sha, embedded in the visible "review started" ack comment.
"""

from __future__ import annotations

from app.github.client import GitHubClient


def marker(head_sha: str) -> str:
    return f"<!-- review-bot:started:{head_sha} -->"


def started_comment(head_sha: str) -> str:
    return (
        "🤖 **AI review started.** Exploring the repository and reviewing this pull "
        "request with a local model — this takes a few minutes. I'll post the review "
        "here when it's ready.\n\n" + marker(head_sha)
    )


def already_started(client: GitHubClient, number: int, head_sha: str) -> bool:
    mark = marker(head_sha)
    return any(mark in body for body in client.issue_comment_bodies(number))
