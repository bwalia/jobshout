"""Render findings into a GitHub review payload."""

from __future__ import annotations

from typing import Any

from app.reviewer.schema import Issue, Review

_SEVERITY_LABEL = {"high": "High", "medium": "Medium", "low": "Low"}

# Plain-language names for the two questions that matter most.
_CATEGORY_LABEL = {
    "breaks": "Breaks something that works today",
    "intent": "Will not do what this PR is meant to do",
}


def _tag(issue: Issue) -> str:
    """One short line of context above a comment, not a paragraph."""
    label = _CATEGORY_LABEL.get(issue.category)
    severity = f"{_SEVERITY_LABEL[issue.severity].lower()} priority"
    return f"{label} · {severity}" if label else severity


def _comment_body(issue: Issue) -> str:
    # Kept deliberately short and scannable: a header, a one-line tag, the problem,
    # the fix. Long prose in a PR comment does not get read.
    parts = [
        f"**{issue.title}**",
        "",
        f"*{_tag(issue)}*",
        "",
        issue.reason,
    ]
    if issue.suggestion:
        parts += ["", f"**Fix:** {issue.suggestion}"]
    return "\n".join(parts)


def build_comments(review: Review) -> list[dict[str, Any]]:
    """Inline comments, anchored to the new side of the diff."""
    return [
        {
            "path": issue.file,
            "line": issue.line,
            "side": "RIGHT",
            "body": _comment_body(issue),
        }
        for issue in review.inline
    ]


def _headline(review: Review) -> list[str]:
    """The first thing a human sees: merge it, or fix it first."""
    blocking = review.blocking
    if review.decision == "FIX":
        count = len(blocking)
        thing = "issue" if count == 1 else "issues"
        return [f"# 🛑 FIX", "", f"**{count} {thing} to fix before merging.**"]
    return ["# ✅ MERGE", "", "**Nothing blocking found.**"]


def build_summary(review: Review, model: str) -> str:
    parts = _headline(review)
    parts += ["", f"**Will this do what it's meant to?** {review.verdict}", "", review.summary]

    # GitHub renders inline comments in file/line order, so this is the only place
    # we control what gets read first.
    blocking = review.blocking
    if blocking:
        parts += ["", "## Fix these first", ""]
        for index, issue in enumerate(blocking, 1):
            location = f"`{issue.file}`" + (f" line {issue.line}" if issue.line else "")
            parts.append(f"**{index}. {issue.title}**")
            parts.append(f"- Where: {location}")
            parts.append(f"- Problem: {issue.reason}")
            if issue.suggestion:
                parts.append(f"- Fix: {issue.suggestion}")
            parts.append("")

    # Real findings that are worth saying but should not hold up a merge.
    minor = [issue for issue in review.inline + review.orphaned if issue not in blocking]
    if minor:
        parts += ["", "## Also worth a look", ""]
        for issue in minor:
            location = f"`{issue.file}`" + (f" line {issue.line}" if issue.line else "")
            parts.append(f"- **{issue.title}** — {location}. {issue.reason}")

    parts += ["", "---", f"<sub>{len(review.inline)} inline comment(s) · reviewed locally by `{model}` via OpenCode + Ollama.</sub>"]
    return "\n".join(parts)
