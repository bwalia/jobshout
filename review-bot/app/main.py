"""CLI entrypoint.

Usage:
    python -m app.main review <PR_NUMBER> [--dry-run]
    python -m app.main prime [--force]           # (re)build the cached repo map
    python -m app.main watch [--interval N] [--once]  # auto-review on review requests
"""

from __future__ import annotations

import argparse
import sys

from app.config.settings import ConfigError, load_settings
from app.github.client import GitHubClient, GitHubError, repo_slug
from app.github.review import build_summary
from app.opencode.runner import OpenCodeError
from app.reviewer.agent import NothingToReview, review_pull_request
from app.reviewer.repo_map import build_map, load_cached
from app.reviewer.workspace import WorkspaceError
from app.watch import DEFAULT_INTERVAL, poll_once, watch


def _log(message: str) -> None:
    print(f"  → {message}", flush=True)


def _print_dry_run(result, model: str) -> None:
    print("\n" + "=" * 72)
    print(build_summary(result.review, model))
    print("=" * 72)
    if result.review.inline:
        print("\nInline comments that would be posted:")
    for issue in result.review.inline:  # already ordered: critical first
        flag = "CRITICAL " if issue.is_critical else ""
        print(f"\n[{flag}{issue.severity.upper()}] {issue.file}:{issue.line} — {issue.title}")
        print(f"  {issue.reason}")
        if issue.suggestion:
            print(f"  What to do: {issue.suggestion}")
    print("\n(dry run — nothing was posted to GitHub)")


def _cmd_review(settings, args) -> int:
    slug = repo_slug(settings.local_repo_path)
    client = GitHubClient(settings.github_token, slug)
    result = review_pull_request(
        settings=settings,
        client=client,
        pr_number=args.pr_number,
        dry_run=args.dry_run,
        on_progress=_log,
    )
    if args.dry_run:
        _print_dry_run(result, settings.model)
    else:
        print(f"\n{result.review.decision} — review posted: {result.url}")
    return 0


def _cmd_prime(settings, args) -> int:
    if not args.force and load_cached(settings):
        print("A cached repo map already exists. Use --force to rebuild it.")
        return 0
    cached = build_map(settings, on_progress=_log)
    print(f"\nRepo map built at {cached.built_sha[:8]} ({len(cached.text)} chars) — cached.")
    return 0


def _cmd_watch(settings, args) -> int:
    slug = repo_slug(settings.local_repo_path)
    client = GitHubClient(settings.github_token, slug)
    if args.once:
        handled = poll_once(settings, client, on_event=_log)
        print(f"\nOne pass complete — {handled} PR(s) handled.")
        return 0
    watch(settings, client, interval=args.interval, on_event=_log)
    return 0


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="review-bot", description="AI PR reviewer (local Ollama + OpenCode)")
    subparsers = parser.add_subparsers(dest="command", required=True)

    review_parser = subparsers.add_parser("review", help="review a pull request")
    review_parser.add_argument("pr_number", type=int)
    review_parser.add_argument(
        "--dry-run",
        action="store_true",
        help="print the review instead of posting it to GitHub",
    )

    prime_parser = subparsers.add_parser("prime", help="(re)build the cached repository map")
    prime_parser.add_argument(
        "--force",
        action="store_true",
        help="rebuild even if a cached map already exists",
    )

    watch_parser = subparsers.add_parser(
        "watch", help="poll for review requests and auto-review those PRs"
    )
    watch_parser.add_argument(
        "--interval", type=int, default=DEFAULT_INTERVAL, help=f"seconds between polls (default {DEFAULT_INTERVAL})"
    )
    watch_parser.add_argument(
        "--once", action="store_true", help="run a single polling pass and exit"
    )

    args = parser.parse_args(argv)

    try:
        settings = load_settings()
        if settings.local_repo_path is None:
            print("error: LOCAL_REPO_PATH is not set.", file=sys.stderr)
            return 1
        if args.command == "prime":
            return _cmd_prime(settings, args)
        if args.command == "watch":
            return _cmd_watch(settings, args)
        return _cmd_review(settings, args)
    except (ConfigError, GitHubError, WorkspaceError, OpenCodeError, NothingToReview) as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
