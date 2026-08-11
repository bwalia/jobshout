"""Minimal GitHub REST client for reading PRs and publishing reviews."""

from __future__ import annotations

import re
from dataclasses import dataclass
from pathlib import Path
from typing import Any

import requests
from git import Repo

API_ROOT = "https://api.github.com"
TIMEOUT = 30

# Matches both git@github.com:owner/name.git and https://github.com/owner/name.git
_REMOTE_RE = re.compile(r"github\.com[:/](?P<owner>[^/]+)/(?P<name>[^/]+?)(?:\.git)?$")


class GitHubError(RuntimeError):
    """Raised when the GitHub API rejects a request."""


@dataclass(frozen=True)
class PullRequest:
    number: int
    title: str
    body: str
    head_sha: str


def repo_slug(repo_path: Path) -> str:
    """Derive "owner/name" from the local repository's origin remote."""
    repo = Repo(repo_path)
    try:
        url = repo.remotes.origin.url
    except (AttributeError, ValueError) as exc:
        raise GitHubError(f"No 'origin' remote in {repo_path}") from exc

    match = _REMOTE_RE.search(url.strip())
    if not match:
        raise GitHubError(f"Could not parse a GitHub repo from origin URL: {url}")
    return f"{match['owner']}/{match['name']}"


class GitHubClient:
    def __init__(self, token: str, slug: str) -> None:
        self.slug = slug
        self._session = requests.Session()
        self._session.headers.update(
            {
                "Authorization": f"Bearer {token}",
                "Accept": "application/vnd.github+json",
                "X-GitHub-Api-Version": "2022-11-28",
            }
        )

    def _request(self, method: str, path: str, **kwargs: Any) -> Any:
        response = self._session.request(method, f"{API_ROOT}{path}", timeout=TIMEOUT, **kwargs)
        if not response.ok:
            raise GitHubError(f"{method} {path} -> {response.status_code}: {response.text[:500]}")
        return response.json()

    def get_pull_request(self, number: int) -> PullRequest:
        data = self._request("GET", f"/repos/{self.slug}/pulls/{number}")
        return PullRequest(
            number=data["number"],
            title=data["title"],
            body=data.get("body") or "",
            head_sha=data["head"]["sha"],
        )

    def list_review_requested(self) -> list[PullRequest]:
        """Open PRs that currently have a reviewer (person or team) requested.

        GitHub keeps a user in `requested_reviewers` until they submit a review, so
        this is a natural signal for "a reviewer was added" without needing an events
        feed. Deduplication (have we already handled this head sha?) is done by the
        caller via a marker comment.
        """
        requested: list[PullRequest] = []
        page = 1
        while True:
            batch = self._request(
                "GET",
                f"/repos/{self.slug}/pulls",
                params={"state": "open", "per_page": 100, "page": page},
            )
            for data in batch:
                if data.get("requested_reviewers") or data.get("requested_teams"):
                    requested.append(
                        PullRequest(
                            number=data["number"],
                            title=data["title"],
                            body=data.get("body") or "",
                            head_sha=data["head"]["sha"],
                        )
                    )
            if len(batch) < 100:
                return requested
            page += 1

    def issue_comment_bodies(self, number: int) -> list[str]:
        """All issue-comment bodies on a PR (used to find our own marker)."""
        bodies: list[str] = []
        page = 1
        while True:
            batch = self._request(
                "GET",
                f"/repos/{self.slug}/issues/{number}/comments",
                params={"per_page": 100, "page": page},
            )
            bodies.extend(c.get("body") or "" for c in batch)
            if len(batch) < 100:
                return bodies
            page += 1

    def post_issue_comment(self, number: int, body: str) -> str:
        """Post a plain PR comment (the 'started' ack and error notices)."""
        data = self._request(
            "POST", f"/repos/{self.slug}/issues/{number}/comments", json={"body": body}
        )
        return data.get("html_url", "")

    def get_changed_files(self, number: int) -> list[dict[str, Any]]:
        """Return changed files, following pagination."""
        files: list[dict[str, Any]] = []
        page = 1
        while True:
            batch = self._request(
                "GET",
                f"/repos/{self.slug}/pulls/{number}/files",
                params={"per_page": 100, "page": page},
            )
            files.extend(batch)
            if len(batch) < 100:
                return files
            page += 1

    def post_review(
        self,
        number: int,
        commit_id: str,
        body: str,
        comments: list[dict[str, Any]],
    ) -> str:
        """Publish a single review carrying all inline comments. Returns its URL."""
        payload = {
            "commit_id": commit_id,
            "body": body,
            "event": "COMMENT",
            "comments": comments,
        }
        data = self._request("POST", f"/repos/{self.slug}/pulls/{number}/reviews", json=payload)
        return data.get("html_url", "")
