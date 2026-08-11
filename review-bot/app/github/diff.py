"""Parse PR patches to find which lines GitHub will accept comments on.

GitHub rejects (422) any inline comment on a line outside the diff hunks, which
would fail the entire review. We map the commentable lines up front and validate
findings against them before posting.
"""

from __future__ import annotations

import re
from dataclasses import dataclass
from typing import Any

# "@@ -12,7 +14,9 @@ def some_function():" -> we need the new-side start (14)
_HUNK_RE = re.compile(r"^@@ -\d+(?:,\d+)? \+(\d+)(?:,\d+)? @@")

# Files above GitHub's diff limit arrive without a patch; nothing is commentable.
MAX_DIFF_CHARS = 60_000


@dataclass(frozen=True)
class ChangedFile:
    path: str
    status: str
    patch: str
    commentable_lines: frozenset[int]


def _commentable_lines(patch: str) -> frozenset[int]:
    """New-side line numbers that appear in the diff (added + context lines).

    Removed lines only exist on the LEFT side, so they are not included.
    """
    lines: set[int] = set()
    new_line = 0

    for raw in patch.splitlines():
        hunk = _HUNK_RE.match(raw)
        if hunk:
            new_line = int(hunk.group(1))
            continue
        if not raw or new_line == 0:
            continue

        marker = raw[0]
        if marker in "+ ":
            lines.add(new_line)
            new_line += 1
        # "-" consumes an old-side line only; "\" is the no-newline marker.

    return frozenset(lines)


def parse_changed_files(raw_files: list[dict[str, Any]]) -> list[ChangedFile]:
    parsed = []
    for entry in raw_files:
        patch = entry.get("patch") or ""
        parsed.append(
            ChangedFile(
                path=entry["filename"],
                status=entry.get("status", "modified"),
                patch=patch,
                commentable_lines=_commentable_lines(patch),
            )
        )
    return parsed


def render_diff(files: list[ChangedFile], budget: int = MAX_DIFF_CHARS) -> str:
    """Render the diff for the prompt, truncating to stay within the model's context."""
    chunks: list[str] = []
    used = 0

    for file in files:
        if not file.patch:
            chunks.append(f"--- {file.path} ({file.status}) ---\n[no diff available]")
            continue

        block = f"--- {file.path} ({file.status}) ---\n{file.patch}"
        if used + len(block) > budget:
            chunks.append(f"--- {file.path} ({file.status}) ---\n[diff truncated to fit context]")
            used = budget
            continue

        chunks.append(block)
        used += len(block)

    return "\n\n".join(chunks)
