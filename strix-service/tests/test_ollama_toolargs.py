"""Ollama/LiteLLM tool-argument parsing — Extra data must not kill a scan."""

import json

import pytest

from app.ollama_toolargs import parse_tool_arguments


def test_plain_object_is_unchanged():
    assert parse_tool_arguments('{"url":"https://int.jobshout.co.uk"}') == {
        "url": "https://int.jobshout.co.uk",
    }


def test_concatenated_objects_keep_the_first():
    # The failure from run 3e7b4dc0: Extra data at column 212. Two objects
    # jammed together is the shape LiteLLM issue 33678 describes.
    raw = '{"url":"https://int.jobshout.co.uk/login"}{"url":"https://int.jobshout.co.uk/api"}'
    assert parse_tool_arguments(raw) == {"url": "https://int.jobshout.co.uk/login"}


def test_dict_passthrough():
    assert parse_tool_arguments({"path": "/tmp"}) == {"path": "/tmp"}


def test_empty_and_none_are_empty_dicts():
    assert parse_tool_arguments(None) == {}
    assert parse_tool_arguments("") == {}
    assert parse_tool_arguments("   ") == {}


def test_trailing_garbage_after_one_object_is_ignored():
    assert parse_tool_arguments('{"a":1} trailing prose') == {"a": 1}


def test_truly_invalid_json_still_raises():
    with pytest.raises(json.JSONDecodeError):
        parse_tool_arguments("not-json")


def test_build_env_puts_patches_on_pythonpath(runner, monkeypatch):
    monkeypatch.delenv("PYTHONPATH", raising=False)
    env = runner.build_env()
    assert env["PYTHONPATH"].endswith("patches") or "/patches:" in env["PYTHONPATH"]
    from pathlib import Path
    assert (Path(env["PYTHONPATH"].split(":")[0]) / "sitecustomize.py").is_file()
