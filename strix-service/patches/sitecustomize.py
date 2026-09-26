"""Loaded by the Strix CLI process when runner.py puts this directory on PYTHONPATH.

LiteLLM 1.97's Ollama chat transformer calls json.loads on tool-call
arguments. Local models (qwen3-coder via Ollama) concatenate two JSON
objects into that string; json.loads then raises Extra data and Strix
exits 1. raw_decode keeps the first object so the scan can continue.
"""

from __future__ import annotations

import json

# Hold the real stdlib loader. Patching transformation.json.loads also
# patches json.loads (same module object), so calling json.loads inside
# the wrapper would recurse until RecursionError — which is worse than
# the Extra data crash this file exists to stop.
_ORIG_LOADS = json.loads


def _tolerant_loads(s, *args, **kwargs):
    if isinstance(s, (dict, list)):
        return s
    if not isinstance(s, (str, bytes, bytearray)):
        return _ORIG_LOADS(s, *args, **kwargs)  # type: ignore[arg-type]
    text = s.decode() if isinstance(s, (bytes, bytearray)) else s
    try:
        return _ORIG_LOADS(text, *args, **kwargs)
    except json.JSONDecodeError as exc:
        if exc.msg != "Extra data":
            raise
        obj, _end = json.JSONDecoder().raw_decode(text)
        return obj


def _install() -> None:
    try:
        import litellm.llms.ollama.chat.transformation as transformation
    except Exception:
        return
    transformation.json.loads = _tolerant_loads  # type: ignore[method-assign]


_install()
