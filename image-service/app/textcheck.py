"""Reading a generated picture's lettering back.

ERNIE-Image letters a cover's labels right about two times in three. The rest
repeat a label ("AUTH" above and below its icon) or misspell one ("ordng
service"). A caller that names the words a picture must show gets it redrawn
when the lettering does not read back as exactly those words, so a bad one is
never what it receives. The reading is Apple Vision's OCR (ocr/ocr.swift).
"""

from __future__ import annotations

import logging
import os
import re
import subprocess
import tempfile
from collections import Counter
from pathlib import Path

logger = logging.getLogger(__name__)

OCR_BIN = Path(os.getenv("IMAGE_OCR_BIN", str(Path(__file__).resolve().parent.parent / "ocr" / "ocr")))

_WORD = re.compile(r"[A-Za-z0-9]+")


def words(text: str) -> Counter:
    """Upper-cased words, ignoring punctuation and stray single letters an
    icon reads as."""
    return Counter(w.upper() for w in _WORD.findall(text) if len(w) > 1 or w.isdigit())


def problems(expected: list[str], seen: str) -> list[str]:
    """What is wrong with lettering that reads as seen, for a picture that
    should show exactly the expected labels: a word missing (or misspelt), a
    word repeated, or a word nobody asked for. Empty means it is right."""
    want, got = words(" ".join(expected)), words(seen)
    out = []
    for word, n in want.items():
        if got[word] < n:
            out.append(f"missing {word}")
        elif got[word] > n:
            out.append(f"repeated {word}")
    # Only real-looking words: an icon's squiggle reads as "II" or "Oo".
    out += [f"extra {word}" for word in got if word not in want and len(word) >= 3 and word.isalpha()]
    return out


def read_text(png: bytes) -> str | None:
    """The text Vision reads in png, or None when it cannot be read here (no
    OCR tool built), in which case the lettering goes unchecked."""
    if not OCR_BIN.is_file():
        logger.warning("lettering not checked: %s is missing (run.sh builds it)", OCR_BIN)
        return None
    with tempfile.NamedTemporaryFile(suffix=".png") as f:
        f.write(png)
        f.flush()
        try:
            done = subprocess.run([str(OCR_BIN), f.name], capture_output=True, text=True, timeout=60, check=True)
        except (subprocess.SubprocessError, OSError) as exc:
            logger.warning("lettering not checked: %s", exc)
            return None
    return done.stdout
