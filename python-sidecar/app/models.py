"""Shared Pydantic request/response models for the sidecar API."""

from __future__ import annotations

from pydantic import BaseModel


class RunRequest(BaseModel):
    """Incoming execution request from the Go backend."""

    execution_id: str
    agent_id: str
    prompt: str
    system_prompt: str = ""
    model: str = ""
    provider: str = ""  # "ollama" | "openai"
    tools: list[str] = []
    config: dict = {}


class ToolCallRecord(BaseModel):
    tool_name: str
    input: dict = {}
    output: str = ""
    error: str | None = None
    duration_ms: int = 0


class StateSnapshotRecord(BaseModel):
    step_number: int
    node_name: str
    state_json: dict = {}


class RunResponse(BaseModel):
    """Response sent back to the Go backend."""

    execution_id: str
    final_answer: str = ""
    iterations: int = 0
    total_tokens: int = 0
    tool_calls: list[ToolCallRecord] = []
    snapshots: list[StateSnapshotRecord] = []
    error: str | None = None
