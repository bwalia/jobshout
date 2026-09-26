"""Parse Ollama/LiteLLM tool-call arguments that are not strict JSON.

qwen3-coder (and other local models) often emit *two* JSON objects jammed
into one `arguments` string, e.g. `{"url":"..."}{"url":"..."}`. LiteLLM
1.97 then does `json.loads` in `OllamaChatConfig.transform_request` and
the whole Strix process dies with `JSONDecodeError: Extra data`.

`json.JSONDecoder.raw_decode` returns the first value and ignores the
rest, which is enough for the current tool call to proceed instead of
taking the scan down.
"""

from __future__ import annotations

import json


def parse_tool_arguments(raw: object) -> dict:
    if raw is None:
        return {}
    if isinstance(raw, dict):
        return raw
    if not isinstance(raw, str):
        return {}
    text = raw.strip()
    if not text:
        return {}
    try:
        obj, _end = json.JSONDecoder().raw_decode(text)
    except json.JSONDecodeError:
        raise
    if isinstance(obj, dict):
        return obj
    if isinstance(obj, list):
        return {"items": obj}
    return {"value": obj}
