"""LangGraph execution runner — builds and executes stateful graph workflows."""

from __future__ import annotations

import logging
import time
from typing import Any, TypedDict

from langchain_core.messages import AIMessage, HumanMessage, SystemMessage
from langgraph.graph import END, StateGraph

from app.config import settings
from app.models import RunRequest, RunResponse, StateSnapshotRecord, ToolCallRecord

logger = logging.getLogger(__name__)


class AgentState(TypedDict):
    """Minimal state passed through the graph."""

    messages: list[Any]
    final_answer: str
    current_step: str
    iterations: int


def _get_llm(req: RunRequest) -> Any:
    """Resolve the LLM backend based on provider."""
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


def _build_default_graph(
    llm: Any,
    system_prompt: str,
) -> StateGraph:
    """Build a default ReAct-style graph: reason → respond."""

    def reason_node(state: AgentState) -> AgentState:
        """First pass: analyze the problem."""
        messages = list(state["messages"])
        messages.insert(
            0,
            SystemMessage(
                content=(system_prompt or "You are a helpful AI assistant.")
                + "\n\nFirst, analyze the problem step by step."
            ),
        )
        response = llm.invoke(messages)
        content = response.content if hasattr(response, "content") else str(response)
        state["messages"].append(AIMessage(content=content))
        state["current_step"] = "reason"
        state["iterations"] += 1
        return state

    def respond_node(state: AgentState) -> AgentState:
        """Second pass: produce the final answer."""
        messages = list(state["messages"])
        messages.append(
            HumanMessage(
                content="Based on your analysis above, provide a clear, actionable final answer."
            )
        )
        response = llm.invoke(messages)
        content = response.content if hasattr(response, "content") else str(response)
        state["final_answer"] = content
        state["current_step"] = "respond"
        state["iterations"] += 1
        return state

    graph = StateGraph(AgentState)
    graph.add_node("reason", reason_node)
    graph.add_node("respond", respond_node)
    graph.set_entry_point("reason")
    graph.add_edge("reason", "respond")
    graph.add_edge("respond", END)

    return graph


def _build_custom_graph(
    llm: Any,
    system_prompt: str,
    graph_def: dict,
) -> StateGraph:
    """Build a graph from a user-supplied definition.

    graph_def format:
    {
        "nodes": ["analyze_logs", "detect_issue", "suggest_fix"],
        "edges": [
            ["analyze_logs", "detect_issue"],
            ["detect_issue", "suggest_fix"]
        ],
        "entry_point": "analyze_logs"
    }
    """
    nodes = graph_def.get("nodes", [])
    edges = graph_def.get("edges", [])
    entry = graph_def.get("entry_point", nodes[0] if nodes else "start")

    graph = StateGraph(AgentState)

    for node_name in nodes:

        def make_node(name: str):
            def node_fn(state: AgentState) -> AgentState:
                prompt_text = (
                    f"{system_prompt}\n\n"
                    f"You are executing step: {name}\n"
                    f"Task context so far:\n"
                )
                for msg in state["messages"]:
                    if hasattr(msg, "content"):
                        prompt_text += f"- {msg.content[:200]}\n"

                response = llm.invoke(
                    [
                        SystemMessage(content=prompt_text),
                        HumanMessage(content=f"Execute step '{name}' and provide your output."),
                    ]
                )
                content = response.content if hasattr(response, "content") else str(response)
                state["messages"].append(AIMessage(content=f"[{name}]: {content}"))
                state["final_answer"] = content
                state["current_step"] = name
                state["iterations"] += 1
                return state

            return node_fn

        graph.add_node(node_name, make_node(node_name))

    graph.set_entry_point(entry)

    for src, dst in edges:
        graph.add_edge(src, dst)

    # Connect the last node to END if not explicitly connected.
    if nodes:
        last_node = nodes[-1]
        terminal_sources = {e[0] for e in edges}
        terminal_targets = {e[1] for e in edges}
        leaf_nodes = set(nodes) - terminal_sources
        if not leaf_nodes:
            leaf_nodes = {last_node}
        for leaf in leaf_nodes:
            if leaf not in terminal_sources:
                graph.add_edge(leaf, END)

    return graph


def run(req: RunRequest) -> RunResponse:
    """Execute a LangGraph workflow and return the result."""
    start = time.monotonic()
    snapshots: list[StateSnapshotRecord] = []

    try:
        llm = _get_llm(req)
        system_prompt = req.system_prompt or "You are a helpful AI assistant."

        # Build the graph.
        graph_def = req.config.get("graph_definition")
        if graph_def and isinstance(graph_def, dict) and graph_def.get("nodes"):
            graph = _build_custom_graph(llm, system_prompt, graph_def)
        else:
            graph = _build_default_graph(llm, system_prompt)

        compiled = graph.compile()

        # Initial state.
        initial_state: AgentState = {
            "messages": [HumanMessage(content=req.prompt)],
            "final_answer": "",
            "current_step": "",
            "iterations": 0,
        }

        # Execute the graph.
        step_num = 0
        final_state = initial_state
        for state in compiled.stream(initial_state):
            # state is a dict of {node_name: updated_state}
            for node_name, node_state in state.items():
                step_num += 1
                final_state = node_state
                snapshots.append(
                    StateSnapshotRecord(
                        step_number=step_num,
                        node_name=node_name,
                        state_json={
                            "current_step": node_state.get("current_step", ""),
                            "iterations": node_state.get("iterations", 0),
                            "final_answer_preview": (
                                node_state.get("final_answer", "")[:200]
                            ),
                        },
                    )
                )

        elapsed_ms = int((time.monotonic() - start) * 1000)
        final_answer = final_state.get("final_answer", "")
        iterations = final_state.get("iterations", step_num)

        return RunResponse(
            execution_id=req.execution_id,
            final_answer=final_answer,
            iterations=iterations,
            total_tokens=0,
            tool_calls=[
                ToolCallRecord(
                    tool_name="langgraph_execute",
                    input={"prompt": req.prompt[:200]},
                    output=final_answer[:500],
                    duration_ms=elapsed_ms,
                )
            ],
            snapshots=snapshots,
        )

    except Exception as exc:
        logger.exception("LangGraph execution failed")
        return RunResponse(
            execution_id=req.execution_id,
            error=str(exc),
            snapshots=snapshots,
        )
