"""Check out PR code from the existing local repository.

Uses a git worktree rather than a clone: it reuses the local object store (no
network clone per request) and leaves the user's working directory untouched.
"""

from __future__ import annotations

import shutil
import tempfile
from contextlib import contextmanager
from pathlib import Path
from typing import Iterator

from git import GitCommandError, Repo


class WorkspaceError(RuntimeError):
    """Raised when the PR code cannot be checked out."""


@contextmanager
def _worktree(repo: Repo, prefix: str, ref: str) -> Iterator[Path]:
    """Add a detached worktree at ref, yield it, and always clean it up."""
    workdir = Path(tempfile.mkdtemp(prefix=prefix))
    try:
        try:
            repo.git.worktree("add", "--detach", "--force", str(workdir), ref)
        except GitCommandError as exc:
            raise WorkspaceError(f"Could not create worktree at {ref[:8]}: {exc}") from exc
        yield workdir
    finally:
        try:
            repo.git.worktree("remove", "--force", str(workdir))
        except GitCommandError:
            shutil.rmtree(workdir, ignore_errors=True)  # fall back to a plain delete
        repo.git.worktree("prune")


@contextmanager
def pr_worktree(repo_path: Path, pr_number: int, head_sha: str) -> Iterator[Path]:
    repo = Repo(repo_path)
    # Fetch the PR head; the local repo may not have the contributor's branch.
    try:
        repo.git.fetch("origin", f"pull/{pr_number}/head", "--quiet")
    except GitCommandError as exc:
        raise WorkspaceError(f"Could not fetch PR #{pr_number} from origin: {exc}") from exc

    with _worktree(repo, f"review-bot-pr{pr_number}-", head_sha) as workdir:
        yield workdir


@contextmanager
def head_worktree(repo_path: Path) -> Iterator[tuple[Path, str]]:
    """A detached worktree at the repo's current HEAD, plus that HEAD sha.

    Used to build the repo map from a stable snapshot without disturbing the
    user's working directory.
    """
    repo = Repo(repo_path)
    head_sha = repo.head.commit.hexsha
    with _worktree(repo, "review-bot-map-", head_sha) as workdir:
        yield workdir, head_sha
