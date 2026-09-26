"""Serialized background jobs for reviews and repo-map priming.

The single worker thread IS the global review lock: worktree cleanup prunes the
whole repository (workspace.py), so two concurrent reviews of one clone can
remove each other's checkouts mid-review. Concurrency stays at exactly 1; extra
jobs wait in the queue and report their position.

Jobs live in memory only. A server restart forgets them — callers polling an
unknown id get a clear "job not found" so they can resubmit.
"""

from __future__ import annotations

import itertools
import queue
import threading
import time
import uuid
from dataclasses import dataclass, field

from app.config.settings import ConfigError, Settings
from app.github.client import GitHubClient, GitHubError
from app.github.markers import already_started, started_comment
from app.opencode.runner import OpenCodeError
from app.repos import RepoNotAllowed, RepoRegistry
from app.reviewer.agent import NothingToReview, ReviewResult, review_pull_request
from app.reviewer.repo_map import build_map, load_cached
from app.reviewer.schema import Issue
from app.reviewer.workspace import WorkspaceError

# Worst observed path: run_review retries once on NoReviewJSON (2 × 900s), plus
# fetch/clone and GitHub round-trips. Cap matches Helm reviewBot.maxRuntime (40m).
JOB_TIMEOUT = 2400

_EXPECTED_ERRORS = (ConfigError, GitHubError, WorkspaceError, OpenCodeError, RepoNotAllowed)


def _clock() -> str:
    return time.strftime("%H:%M:%S")


def _iso(epoch: float | None) -> str | None:
    if epoch is None:
        return None
    return time.strftime("%Y-%m-%dT%H:%M:%S%z", time.localtime(epoch))


@dataclass
class Job:
    id: str
    kind: str  # "review" | "prime"
    params: dict
    seq: int  # submission order; used for queue positions
    state: str = "queued"  # queued | running | done | failed
    stage_log: list[str] = field(default_factory=list)
    result: dict | None = None
    error: str | None = None
    created: float = field(default_factory=time.time)
    started: float | None = None
    finished: float | None = None

    def append_stage(self, message: str) -> None:
        self.stage_log.append(f"{_clock()} {message}")


def issue_to_dict(issue: Issue) -> dict:
    return {
        "severity": issue.severity,
        "category": issue.category,
        "file": issue.file,
        "line": issue.line,
        "title": issue.title,
        "reason": issue.reason,
        "suggestion": issue.suggestion,
        "is_critical": issue.is_critical,
    }


def review_to_dict(result: ReviewResult) -> dict:
    """Manual serializer: decision/blocking are computed properties, which
    dataclasses.asdict() silently drops."""
    review = result.review
    return {
        "pr_number": result.pull_request.number,
        "pr_title": result.pull_request.title,
        "head_sha": result.pull_request.head_sha,
        "decision": review.decision,
        "verdict": review.verdict,
        "summary": review.summary,
        "blocking": [issue_to_dict(i) for i in review.blocking],
        "inline": [issue_to_dict(i) for i in review.inline],
        "orphaned": [issue_to_dict(i) for i in review.orphaned],
        "url": result.url,
    }


class JobRunner:
    def __init__(self, settings: Settings, registry: RepoRegistry, timeout: float = JOB_TIMEOUT) -> None:
        self._settings = settings
        self._registry = registry
        self._timeout = timeout
        self._jobs: dict[str, Job] = {}
        self._queue: queue.Queue[str] = queue.Queue()
        self._lock = threading.Lock()
        self._seq = itertools.count()
        self._worker = threading.Thread(target=self._run_forever, name="review-worker", daemon=True)
        self._worker.start()

    # -- submission ---------------------------------------------------------

    def submit_review(
        self,
        repo: str,
        pr_number: int,
        dry_run: bool = False,
        force: bool = False,
        run_ref: str = "",
    ) -> Job:
        self._registry.require(repo)
        if run_ref:
            existing = self.find_by_run_ref(run_ref)
            if existing is not None:
                return existing
        return self._enqueue(
            "review",
            {
                "repo": repo,
                "pr_number": pr_number,
                "dry_run": dry_run,
                "force": force,
                "run_ref": run_ref,
            },
        )

    def find_by_run_ref(self, run_ref: str) -> Job | None:
        """The job submitted with this run_ref, if any. Used as an idempotency key."""
        if not run_ref:
            return None
        with self._lock:
            for job in self._jobs.values():
                if job.kind == "review" and job.params.get("run_ref") == run_ref:
                    return job
        return None

    def submit_prime(self, repo: str, force: bool = False) -> Job:
        self._registry.require(repo)
        return self._enqueue("prime", {"repo": repo, "force": force})

    def _enqueue(self, kind: str, params: dict) -> Job:
        job = Job(id=uuid.uuid4().hex[:8], kind=kind, params=params, seq=next(self._seq))
        job.append_stage(f"Queued {kind} job for {params['repo']}")
        with self._lock:
            self._jobs[job.id] = job
        self._queue.put(job.id)
        return job

    # -- inspection ---------------------------------------------------------

    def get(self, job_id: str) -> Job | None:
        with self._lock:
            return self._jobs.get(job_id)

    def queue_position(self, job: Job) -> int | None:
        """Jobs ahead of this one (a running job counts). 0 = starts next."""
        if job.state != "queued":
            return None
        with self._lock:
            running = any(j.state == "running" for j in self._jobs.values())
            ahead = sum(1 for j in self._jobs.values() if j.state == "queued" and j.seq < job.seq)
        return ahead + (1 if running else 0)

    def snapshot(self, job_id: str) -> dict | None:
        job = self.get(job_id)
        if job is None:
            return None
        snap: dict = {
            "job_id": job.id,
            "kind": job.kind,
            "params": job.params,
            "state": job.state,
            "stage_log": list(job.stage_log),
            "created": _iso(job.created),
            "started": _iso(job.started),
            "finished": _iso(job.finished),
        }
        position = self.queue_position(job)
        if position is not None:
            snap["queue_position"] = position
        if job.result is not None:
            snap["result"] = job.result
        if job.error is not None:
            snap["error"] = job.error
        return snap

    # -- execution ----------------------------------------------------------

    def _run_forever(self) -> None:
        while True:
            job_id = self._queue.get()
            job = self._jobs[job_id]
            job.state = "running"
            job.started = time.time()
            job.append_stage(f"Started {job.kind} job")

            # The body runs in its own thread so a wedged review cannot stall the
            # queue forever. On timeout the job is failed and the worker moves on;
            # the orphaned body dies at the underlying subprocess timeouts.
            body = threading.Thread(target=self._execute, args=(job,), daemon=True)
            body.start()
            body.join(self._timeout)
            if body.is_alive():
                job.error = f"Timed out after {int(self._timeout)}s"
                job.append_stage(job.error)
                self._finish(job, "failed")
            self._queue.task_done()

    def _execute(self, job: Job) -> None:
        try:
            if job.kind == "review":
                self._run_review(job)
            else:
                self._run_prime(job)
        except NothingToReview as exc:
            # Same semantics as the watcher: not an error, just nothing to do.
            job.result = {"nothing_to_review": True, "detail": str(exc)}
            job.append_stage(str(exc))
            self._finish(job, "done")
        except _EXPECTED_ERRORS as exc:
            job.error = f"{type(exc).__name__}: {exc}"
            job.append_stage(f"Failed — {job.error}")
            self._finish(job, "failed")
        except Exception as exc:  # a bug must surface in the job, not kill the worker
            job.error = f"Unexpected {type(exc).__name__}: {exc}"
            job.append_stage(f"Failed — {job.error}")
            self._finish(job, "failed")
        else:
            self._finish(job, "done")

    def _finish(self, job: Job, state: str) -> None:
        if job.state != "running":
            return  # already failed by timeout; a late-finishing body must not overwrite
        job.state = state
        job.finished = time.time()

    def _run_review(self, job: Job) -> None:
        params = job.params
        slug, number = params["repo"], params["pr_number"]
        client = GitHubClient(self._settings.github_token, slug)

        pull_request = client.get_pull_request(number)
        job.append_stage(f"PR #{number} head is {pull_request.head_sha[:8]}")

        if not params["force"] and already_started(client, number, pull_request.head_sha):
            job.append_stage("A review for this head commit was already started — skipping (use force to re-review)")
            job.result = {"already_started": True, "head_sha": pull_request.head_sha}
            return

        if not params["dry_run"]:
            client.post_issue_comment(number, started_comment(pull_request.head_sha))
            job.append_stage("Posted 'review started' ack (dedup marker)")

        settings = self._registry.settings_for(slug)  # clones/fetches on demand
        job.append_stage(f"Repo ready at {settings.local_repo_path}")

        result = review_pull_request(
            settings=settings,
            client=client,
            pr_number=number,
            dry_run=params["dry_run"],
            on_progress=job.append_stage,
        )
        job.result = review_to_dict(result)

    def _run_prime(self, job: Job) -> None:
        slug = job.params["repo"]
        settings = self._registry.settings_for(slug)
        job.append_stage(f"Repo ready at {settings.local_repo_path}")

        if not job.params["force"]:
            cached = load_cached(settings)
            if cached is not None:
                job.append_stage("A cached repo map already exists — skipping (use force to rebuild)")
                job.result = {"primed": False, "cached": True, "built_sha": cached.built_sha}
                return

        cached = build_map(settings, on_progress=job.append_stage)
        job.result = {"primed": True, "built_sha": cached.built_sha, "chars": len(cached.text)}
