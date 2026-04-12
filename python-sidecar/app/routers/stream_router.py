"""SSE streaming endpoints for LangChain and LangGraph execution."""

import json
import logging
import time
from typing import Any, AsyncGenerator

from fastapi import APIRouter, Depends, HTTPException, Request
from fastapi.responses import StreamingResponse

from app.config import settings
from app.models import RunRequest

logger = logging.getLogger(__name__)

router = APIRouter()


def _verify_secret(request: Request) -> None:
    secret = request.headers.get("X-Sidecar-Secret", "")
    if secret != settings.sidecar_secret:
        raise HTTPException(status_code=401, detail="Invalid sidecar secret")


def _sse_event(event_type: str, data: Any) -> str:
    """Format a single SSE event."""
    payload = json.dumps({"type": event_type, "data": data})
    return f"data: {payload}\n\n"


def _get_llm(req: RunRequest) -> Any:
    """Resolve the LLM backend."""
    provider = req.provider or "ollama"
    model_name = req.model

    if provider == "openai":
        from langchain_openai import ChatOpenAI

        return ChatOpenAI(
            model=model_name or settings.openai_default_model,
            api_key=settings.openai_api_key,
            base_url=settings.openai_base_url,
            temperature=0.2,
        )

    from langchain_ollama import ChatOllama

    ollama_url = req.config.get("ollama_base_url", settings.ollama_base_url)
    return ChatOllama(
        model=model_name or settings.ollama_default_model,
        base_url=ollama_url,
        temperature=0.2,
    )


async def _stream_langchain(req: RunRequest) -> AsyncGenerator[str, None]:
    """Stream LangChain execution events."""
    from langchain_core.messages import HumanMessage, SystemMessage

    try:
        llm = _get_llm(req)
        messages: list[Any] = []
        if req.system_prompt:
            messages.append(SystemMessage(content=req.system_prompt))
        messages.append(HumanMessage(content=req.prompt))

        yield _sse_event("thought", {"iteration": 1, "thought": "Processing with LangChain..."})

        start = time.monotonic()
        result = llm.invoke(messages)
        elapsed_ms = int((time.monotonic() - start) * 1000)

        content = result.content if hasattr(result, "content") else str(result)

        yield _sse_event("tool_result", {
            "tool_name": "langchain_invoke",
            "output": content[:500],
            "duration_ms": elapsed_ms,
        })

        yield _sse_event("final_answer", {
            "answer": content,
            "total_tokens": 0,
            "iterations": 1,
        })

    except Exception as exc:
        logger.exception("LangChain streaming failed")
        yield _sse_event("error", {"message": str(exc)})

    yield "data: [DONE]\n\n"


async def _stream_langgraph(req: RunRequest) -> AsyncGenerator[str, None]:
    """Stream LangGraph execution events with per-node updates."""
    from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
    from langgraph.graph import END, StateGraph

    try:
        llm = _get_llm(req)
        system_prompt = req.system_prompt or "You are a helpful AI assistant."

        # Check for custom graph definition.
        graph_def = req.config.get("graph_definition")
        if graph_def and isinstance(graph_def, dict) and graph_def.get("nodes"):
            nodes = graph_def.get("nodes", [])
            edges = graph_def.get("edges", [])
            entry = graph_def.get("entry_point", nodes[0] if nodes else "start")
        else:
            nodes = ["reason", "respond"]
            edges = [["reason", "respond"]]
            entry = "reason"

        from typing import TypedDict

        class AgentState(TypedDict):
            messages: list[Any]
            final_answer: str
            current_step: str
            iterations: int

        graph = StateGraph(AgentState)

        for node_name in nodes:
            def make_node(name: str):
                def node_fn(state: AgentState) -> AgentState:
                    prompt_text = f"{system_prompt}\n\nExecute step: {name}"
                    for msg in state.get("messages", []):
                        if hasattr(msg, "content"):
                            prompt_text += f"\n- {msg.content[:200]}"

                    response = llm.invoke([
                        SystemMessage(content=prompt_text),
                        HumanMessage(content=f"Execute step '{name}' now."),
                    ])
                    content = response.content if hasattr(response, "content") else str(response)
                    msgs = list(state.get("messages", []))
                    msgs.append(AIMessage(content=f"[{name}]: {content}"))
                    return {
                        "messages": msgs,
                        "final_answer": content,
                        "current_step": name,
                        "iterations": state.get("iterations", 0) + 1,
                    }
                return node_fn
            graph.add_node(node_name, make_node(node_name))

        graph.set_entry_point(entry)
        for src, dst in edges:
            graph.add_edge(src, dst)

        # Connect leaf nodes to END.
        source_nodes = {e[0] for e in edges}
        for n in nodes:
            if n not in source_nodes:
                graph.add_edge(n, END)

        compiled = graph.compile()

        initial_state: AgentState = {
            "messages": [HumanMessage(content=req.prompt)],
            "final_answer": "",
            "current_step": "",
            "iterations": 0,
        }

        step_num = 0
        final_state = initial_state
        for state in compiled.stream(initial_state):
            for node_name, node_state in state.items():
                step_num += 1
                final_state = node_state

                yield _sse_event("node_start", {
                    "node_name": node_name,
                    "step_number": step_num,
                })

                yield _sse_event("node_end", {
                    "node_name": node_name,
                    "step_number": step_num,
                    "state": {
                        "current_step": node_state.get("current_step", ""),
                        "iterations": node_state.get("iterations", 0),
                        "final_answer_preview": node_state.get("final_answer", "")[:200],
                    },
                })

        final_answer = final_state.get("final_answer", "")
        yield _sse_event("final_answer", {
            "answer": final_answer,
            "total_tokens": 0,
            "iterations": final_state.get("iterations", step_num),
        })

    except Exception as exc:
        logger.exception("LangGraph streaming failed")
        yield _sse_event("error", {"message": str(exc)})

    yield "data: [DONE]\n\n"


@router.post("/langchain")
async def stream_langchain(req: RunRequest, _: None = Depends(_verify_secret)):
    """SSE streaming for LangChain execution."""
    return StreamingResponse(
        _stream_langchain(req),
        media_type="text/event-stream",
    )


@router.post("/langgraph")
async def stream_langgraph(req: RunRequest, _: None = Depends(_verify_secret)):
    """SSE streaming for LangGraph execution."""
    return StreamingResponse(
        _stream_langgraph(req),
        media_type="text/event-stream",
    )
