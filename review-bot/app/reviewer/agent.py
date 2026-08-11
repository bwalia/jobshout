"""The reviewer: orchestrates one PR review from fetch to publish."""

from __future__ import annotations

from dataclasses import dataclass

from app.config.settings import Settings
from app.github.client import GitHubClient, PullRequest
from app.github.diff import ChangedFile, parse_changed_files, render_diff
from app.github.review import build_comments, build_summary
from app.opencode.runner import run_review
from app.prompts.review import build_prompt
from app.reviewer.repo_map import map_for_review
from app.reviewer.schema import Review, build_review
from app.reviewer.workspace import pr_worktree

# Files whose content the reviewer should not spend context on.
_SKIP_SUFFIXES = (".lock", ".svg", ".png", ".jpg", ".jpeg", ".gif", ".ico", ".pdf", ".min.js", ".min.css")


class NothingToReview(RuntimeError):
    """Raised when a PR has no reviewable code changes."""


@dataclass(frozen=True)
class ReviewResult:
    pull_request: PullRequest
    review: Review
    url: str | None  # None on a dry run


def _reviewable(files: list[ChangedFile]) -> list[ChangedFile]:
    return [
        file
        for file in files
        if file.status != "removed"
        and file.patch
        and not file.path.lower().endswith(_SKIP_SUFFIXES)
    ]


def _file_list(files: list[ChangedFile]) -> str:
    return "\n".join(f"- {file.path} ({file.status})" for file in files)


def review_pull_request(
    settings: Settings,
    client: GitHubClient,
    pr_number: int,
    dry_run: bool = False,
    on_progress=lambda message: None,
) -> ReviewResult:
    on_progress(f"Fetching PR #{pr_number} from {client.slug}")
    pull_request = client.get_pull_request(pr_number)

    files = _reviewable(parse_changed_files(client.get_changed_files(pr_number)))
    if not files:
        raise NothingToReview(f"PR #{pr_number} has no reviewable code changes.")
    on_progress(f"{len(files)} changed file(s) at {pull_request.head_sha[:8]}")

    # Distilled repo knowledge, if the repo has been primed. Opt-in; never built here.
    cached_map = map_for_review(settings, on_progress)

    prompt = build_prompt(
        title=pull_request.title,
        description=pull_request.body,
        file_list=_file_list(files),
        diff=render_diff(files),
        repo_map=cached_map.text if cached_map else None,
    )

    with pr_worktree(settings.local_repo_path, pr_number, pull_request.head_sha) as workdir:
        on_progress(f"Exploring repository and reviewing with {settings.model} (this takes a few minutes)")
        payload = run_review(settings, workdir, prompt)

    review = build_review(payload, files)
    on_progress(f"{len(review.inline)} inline finding(s), {len(review.orphaned)} summary-only")

    if dry_run:
        return ReviewResult(pull_request=pull_request, review=review, url=None)

    url = client.post_review(
        number=pr_number,
        commit_id=pull_request.head_sha,
        body=build_summary(review, settings.model),
        comments=build_comments(review),
    )
    return ReviewResult(pull_request=pull_request, review=review, url=url)
