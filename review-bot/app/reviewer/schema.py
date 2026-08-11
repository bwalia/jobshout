"""Validate the model's JSON and split findings into inline vs summary-only."""

from __future__ import annotations

from dataclasses import dataclass

from app.github.diff import ChangedFile

VALID_SEVERITIES = ("high", "medium", "low")
_SEVERITY_ORDER = {name: index for index, name in enumerate(VALID_SEVERITIES)}

VALID_CATEGORIES = ("breaks", "intent", "other")
# The two questions that matter most: does it break what works, and will it actually
# do what the PR promises? Everything else ranks below them regardless of severity.
CRITICAL_CATEGORIES = ("breaks", "intent")


@dataclass(frozen=True)
class Issue:
    severity: str
    category: str
    file: str
    line: int
    title: str
    reason: str
    suggestion: str

    @property
    def is_critical(self) -> bool:
        return self.category in CRITICAL_CATEGORIES


@dataclass(frozen=True)
class Review:
    verdict: str  # will this PR do what it set out to do?
    summary: str
    inline: list[Issue]  # anchored to a real diff line
    orphaned: list[Issue]  # real findings we cannot anchor; go in the summary

    @property
    def critical(self) -> list[Issue]:
        """Every critical finding, inline or not, worst first."""
        return sorted(
            [issue for issue in self.inline + self.orphaned if issue.is_critical],
            key=_rank,
        )

    @property
    def blocking(self) -> list[Issue]:
        """Findings that mean this should not merge as-is.

        Either the change breaks something, or it will not actually do what the PR
        promises, or it carries a high-severity problem. Anything else is worth
        saying but is not a reason to hold up a merge.
        """
        return sorted(
            [
                issue
                for issue in self.inline + self.orphaned
                if issue.is_critical or issue.severity == "high"
            ],
            key=_rank,
        )

    @property
    def decision(self) -> str:
        return "FIX" if self.blocking else "MERGE"


def _rank(issue: Issue) -> tuple[int, int]:
    """Critical categories first, then by severity."""
    return (0 if issue.is_critical else 1, _SEVERITY_ORDER[issue.severity])


def _clean(value: object, fallback: str = "") -> str:
    return value.strip() if isinstance(value, str) and value.strip() else fallback


def _coerce_issue(raw: object) -> Issue | None:
    """Build an Issue from model output, tolerating loose types. None if unusable."""
    if not isinstance(raw, dict):
        return None

    title = _clean(raw.get("title"))
    reason = _clean(raw.get("reason"))
    if not title and not reason:
        return None  # nothing to say

    path = _clean(raw.get("file"))
    if not path:
        return None

    try:
        line = int(raw.get("line"))
    except (TypeError, ValueError):
        line = 0

    severity = _clean(raw.get("severity"), "medium").lower()
    if severity not in VALID_SEVERITIES:
        severity = "medium"

    category = _clean(raw.get("category"), "other").lower()
    if category not in VALID_CATEGORIES:
        category = "other"

    return Issue(
        severity=severity,
        category=category,
        file=path,
        line=line,
        title=title or reason[:80],
        reason=reason,
        suggestion=_clean(raw.get("suggestion")),
    )


def build_review(payload: dict, files: list[ChangedFile]) -> Review:
    """Validate findings and route each to an inline comment or the summary."""
    line_map = {file.path: file.commentable_lines for file in files}

    inline: list[Issue] = []
    orphaned: list[Issue] = []

    raw_issues = payload.get("issues")
    if not isinstance(raw_issues, list):
        raw_issues = []

    for raw in raw_issues:
        issue = _coerce_issue(raw)
        if issue is None:
            continue
        # An unknown path or an un-commentable line would 422 the whole review,
        # so the finding is kept but moved into the summary instead.
        if issue.line in line_map.get(issue.file, frozenset()):
            inline.append(issue)
        else:
            orphaned.append(issue)

    inline.sort(key=_rank)
    orphaned.sort(key=_rank)

    return Review(
        verdict=_clean(payload.get("verdict"), "No verdict provided."),
        summary=_clean(payload.get("summary"), "No summary provided."),
        inline=inline,
        orphaned=orphaned,
    )
