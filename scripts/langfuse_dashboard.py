#!/usr/bin/env python3
"""Provision the JobShout LLM Observability dashboard in Langfuse.

Dashboards live in Langfuse's database, so a fresh deployment starts without
one. This script is the dashboard-as-code answer: run it once against any
Langfuse (the local compose profile, or a shared deployment) and it creates
every widget plus the dashboard that arranges them. Idempotent — widgets are
matched by name and reused, and an existing dashboard is left alone unless
--force is given.

Uses only the standard library, and the same public API keys the sidecar
traces with (HTTP Basic: public key as user, secret key as password). The
dashboard endpoints are Langfuse's "unstable" API surface, the only one that
exposes dashboard CRUD.

    ./scripts/langfuse_dashboard.py                # local compose defaults
    ./scripts/langfuse_dashboard.py --host https://langfuse.example.com \
        --public-key pk-... --secret-key sk-...
"""

import argparse
import base64
import json
import os
import sys
import urllib.error
import urllib.request

DASHBOARD_NAME = "JobShout LLM Observability"
DASHBOARD_DESCRIPTION = (
    "LLM traffic from the python-sidecar: volume, tokens, cost, latency and "
    "errors, sliced by model, agent and engine."
)

# Every widget reads the observations view; GENERATION-filtered ones count
# model calls only, so LangGraph's span-per-node doesn't inflate the numbers.
GENERATIONS_ONLY = [
    {"column": "type", "operator": "=", "type": "string", "value": "GENERATION", "key": None}
]

WIDGETS = [
    {
        "name": "LLM calls",
        "description": "Total model generations in the selected window.",
        "view": "observations",
        "dimensions": [],
        "metrics": [{"measure": "count", "agg": "count"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "NUMBER",
        "chartConfig": {"type": "NUMBER"},
        "placement": {"x": 0, "y": 0, "width": 4, "height": 3},
    },
    {
        "name": "Total tokens",
        "description": "Input + output tokens across all generations.",
        "view": "observations",
        "dimensions": [],
        "metrics": [{"measure": "totalTokens", "agg": "sum"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "NUMBER",
        "chartConfig": {"type": "NUMBER"},
        "placement": {"x": 4, "y": 0, "width": 4, "height": 3},
    },
    {
        "name": "Total cost (USD)",
        "description": "Model cost — non-zero once models have prices configured.",
        "view": "observations",
        "dimensions": [],
        "metrics": [{"measure": "totalCost", "agg": "sum"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "NUMBER",
        "chartConfig": {"type": "NUMBER"},
        "placement": {"x": 8, "y": 0, "width": 4, "height": 3},
    },
    {
        "name": "LLM calls over time by model",
        "description": "Generation volume per model.",
        "view": "observations",
        "dimensions": [{"field": "providedModelName"}],
        "metrics": [{"measure": "count", "agg": "count"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "BAR_TIME_SERIES",
        "chartConfig": {"type": "BAR_TIME_SERIES"},
        "placement": {"x": 0, "y": 3, "width": 6, "height": 5},
    },
    {
        "name": "p95 latency by model",
        "description": "95th-percentile generation latency per model.",
        "view": "observations",
        "dimensions": [{"field": "providedModelName"}],
        "metrics": [{"measure": "latency", "agg": "p95"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "LINE_TIME_SERIES",
        "chartConfig": {"type": "LINE_TIME_SERIES"},
        "placement": {"x": 6, "y": 3, "width": 6, "height": 5},
    },
    {
        "name": "Calls by agent",
        "description": "Generations per JobShout agent (traced as the Langfuse user).",
        "view": "observations",
        "dimensions": [{"field": "userId"}],
        "metrics": [{"measure": "count", "agg": "count"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "HORIZONTAL_BAR",
        "chartConfig": {"type": "HORIZONTAL_BAR", "row_limit": 10},
        "placement": {"x": 0, "y": 8, "width": 6, "height": 5},
    },
    {
        "name": "Calls by engine",
        "description": "langchain vs langgraph, blocking vs streaming (trace name).",
        "view": "observations",
        "dimensions": [{"field": "traceName"}],
        "metrics": [{"measure": "count", "agg": "count"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "PIE",
        "chartConfig": {"type": "PIE", "row_limit": 10},
        "placement": {"x": 6, "y": 8, "width": 6, "height": 5},
    },
    {
        "name": "Tokens over time",
        "description": "Token throughput across all models.",
        "view": "observations",
        "dimensions": [],
        "metrics": [{"measure": "totalTokens", "agg": "sum"}],
        "filters": GENERATIONS_ONLY,
        "chartType": "AREA_TIME_SERIES",
        "chartConfig": {"type": "AREA_TIME_SERIES"},
        "placement": {"x": 0, "y": 13, "width": 6, "height": 4},
    },
    {
        "name": "Errors over time",
        "description": "Observations that ended at ERROR level.",
        "view": "observations",
        "dimensions": [],
        "metrics": [{"measure": "count", "agg": "count"}],
        "filters": [
            {"column": "level", "operator": "=", "type": "string", "value": "ERROR", "key": None}
        ],
        "chartType": "LINE_TIME_SERIES",
        "chartConfig": {"type": "LINE_TIME_SERIES"},
        "placement": {"x": 6, "y": 13, "width": 6, "height": 4},
    },
]


class Api:
    def __init__(self, host: str, public_key: str, secret_key: str):
        self.base = host.rstrip("/") + "/api/public/unstable"
        token = base64.b64encode(f"{public_key}:{secret_key}".encode()).decode()
        self.auth = f"Basic {token}"

    def call(self, method: str, path: str, body: dict | None = None) -> dict:
        req = urllib.request.Request(
            self.base + path,
            method=method,
            data=json.dumps(body).encode() if body is not None else None,
            headers={"Authorization": self.auth, "Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                raw = resp.read()
                return json.loads(raw) if raw else {}
        except urllib.error.HTTPError as err:
            detail = err.read().decode(errors="replace")
            sys.exit(f"{method} {path} failed: HTTP {err.code}\n{detail}")


def paged(api: Api, path: str) -> list[dict]:
    items, page = [], 1
    while True:
        data = api.call("GET", f"{path}?page={page}&limit=50")
        items.extend(data.get("data", []))
        meta = data.get("meta", {})
        if page >= meta.get("totalPages", 1):
            return items
        page += 1


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default=os.environ.get("LANGFUSE_HOST", "http://localhost:3002"))
    parser.add_argument(
        "--public-key", default=os.environ.get("LANGFUSE_PUBLIC_KEY", "pk-lf-jobshout-local")
    )
    parser.add_argument(
        "--secret-key", default=os.environ.get("LANGFUSE_SECRET_KEY", "sk-lf-jobshout-local")
    )
    parser.add_argument(
        "--force", action="store_true", help="recreate the dashboard if it already exists"
    )
    args = parser.parse_args()

    api = Api(args.host, args.public_key, args.secret_key)

    existing = [d for d in paged(api, "/dashboards") if d.get("name") == DASHBOARD_NAME]
    if existing and not args.force:
        print(f"Dashboard '{DASHBOARD_NAME}' already exists ({existing[0]['id']}); use --force to recreate.")
        return
    for dash in existing:
        api.call("DELETE", f"/dashboards/{dash['id']}")
        print(f"Deleted existing dashboard {dash['id']}")

    widgets_by_name = {w.get("name"): w for w in paged(api, "/dashboard-widgets")}

    dashboard = api.call(
        "POST",
        "/dashboards",
        {"name": DASHBOARD_NAME, "description": DASHBOARD_DESCRIPTION},
    )
    dash_id = dashboard["id"]
    print(f"Created dashboard {dash_id}")

    for spec in WIDGETS:
        placement = spec["placement"]
        body = {k: v for k, v in spec.items() if k != "placement"}
        widget = widgets_by_name.get(spec["name"])
        if widget is None:
            widget = api.call("POST", "/dashboard-widgets", body)
            print(f"  created widget '{spec['name']}' ({widget['id']})")
        else:
            print(f"  reusing widget '{spec['name']}' ({widget['id']})")
        api.call(
            "POST",
            f"/dashboards/{dash_id}/placements",
            {"type": "widget", "widgetId": widget["id"], **placement},
        )

    print(f"Done — open {args.host}/project/<project-id>/dashboards to view it.")


if __name__ == "__main__":
    main()
