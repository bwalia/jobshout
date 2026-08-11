"""Poll a repository for review requests and review those PRs automatically.

Flow per PR that has a reviewer requested:
  1. Post an immediate "review started" comment (so the ack is instant).
  2. Run the review (a few minutes).
  3. Post the review (summary + inline comments) via the Review API.

Deduplication uses a hidden marker in the started comment, keyed by the PR's head
commit. That means: one review per commit, survives restarts with no local state,
and a new push (new head sha) triggers a fresh review. There is no database.
"""

from __future__ import annotations

import time

from app.config.settings import Settings
from app.github.client import GitHubClient, GitHubError
from app.opencode.runner import OpenCodeError
from app.reviewer.agent import NothingToReview, review_pull_request
from app.reviewer.workspace import WorkspaceError

DEFAULT_INTERVAL = 30  # seconds between polls; GitHub's authenticated limit is ample


def _marker(head_sha: str) -> str:
    return f"<!-- review-bot:started:{head_sha} -->"


def _started_comment(head_sha: str) -> str:
    return (
        "🤖 **AI review started.** Exploring the repository and reviewing this pull "
        "request with a local model — this takes a few minutes. I'll post the review "
        "here when it's ready.\n\n" + _marker(head_sha)
    )


def _already_started(client: GitHubClient, number: int, head_sha: str) -> bool:
    marker = _marker(head_sha)
    return any(marker in body for body in client.issue_comment_bodies(number))


def _handle_pr(settings: Settings, client: GitHubClient, pr, on_event) -> None:
    """Ack, review, and post for a single PR. Idempotent per head sha."""
    if _already_started(client, pr.number, pr.head_sha):
        return  # this exact commit has already been picked up

    on_event(f"PR #{pr.number}: review requested at {pr.head_sha[:8]} — starting")
    client.post_issue_comment(pr.number, _started_comment(pr.head_sha))

    try:
        result = review_pull_request(
            settings=settings,
            client=client,
            pr_number=pr.number,
            dry_run=False,
            on_progress=lambda m: on_event(f"PR #{pr.number}: {m}"),
        )
    except NothingToReview:
        client.post_issue_comment(pr.number, "🤖 Nothing reviewable in this PR (no code changes).")
        on_event(f"PR #{pr.number}: nothing to review")
        return
    except (OpenCodeError, WorkspaceError, GitHubError) as exc:
        # The started marker stays, so we do not retry this same commit in a loop.
        # A new push (new head sha) will trigger a fresh attempt.
        client.post_issue_comment(
            pr.number,
            f"🤖 The AI review could not complete: {exc}\n\nPush a new commit to retry.",
        )
        on_event(f"PR #{pr.number}: review failed — {exc}")
        return

    on_event(f"PR #{pr.number}: {result.review.decision} — posted {result.url}")


def poll_once(settings: Settings, client: GitHubClient, on_event=lambda m: None) -> int:
    """One polling pass. Returns how many PRs were newly handled."""
    prs = client.list_review_requested()
    handled = 0
    for pr in prs:
        # Re-check the marker inside the loop; a long review may have elapsed.
        if _already_started(client, pr.number, pr.head_sha):
            continue
        _handle_pr(settings, client, pr, on_event)
        handled += 1
    return handled


def watch(
    settings: Settings,
    client: GitHubClient,
    interval: int = DEFAULT_INTERVAL,
    on_event=lambda m: None,
) -> None:
    """Poll forever. Stops cleanly on Ctrl-C."""
    on_event(f"Watching {client.slug} for review requests (every {interval}s). Ctrl-C to stop.")
    while True:
        try:
            poll_once(settings, client, on_event)
        except GitHubError as exc:
            on_event(f"poll error (will retry): {exc}")  # one bad poll should not kill the watcher
        try:
            time.sleep(interval)
        except KeyboardInterrupt:
            on_event("stopped")
            return
