#!/usr/bin/env python3
"""Register cost estimates for the local Ollama models in Langfuse.

Langfuse only prices generations whose model matches a model definition, and
the workstation's Ollama models are not in its built-in price list — so the
dashboard's cost widgets read $0.00. This registers per-token price estimates
for them, after which every *newly ingested* generation gets a cost.

These are electricity-cost estimates for Apple-silicon inference, not invoices:

    ~80 W sustained draw while generating, at ~$0.30/kWh  →  ~$0.024/hour
    30B-class (q4): ~40 tok/s decode, ~10x that for prompt ingest
        output: 1M tok ÷ 40 tok/s ≈ 6.9 h  →  ~$0.17 / 1M tok
        input:                                 ~$0.017 / 1M tok
    8B-class: roughly 3x the speed  →  a third of the price

Hardware amortization is deliberately excluded, so treat the totals as a
lower bound. Tune PRICE_* below to your machine and tariff.

    ./scripts/langfuse_models.py            # local compose defaults
    ./scripts/langfuse_models.py --host https://langfuse.example.com \
        --public-key pk-... --secret-key sk-...
"""

import argparse
import base64
import json
import os
import sys
import urllib.error
import urllib.request

# USD per token (Langfuse prices are per single token).
PRICE_30B_INPUT = 1.7e-08
PRICE_30B_OUTPUT = 1.7e-07
PRICE_8B_INPUT = 6.0e-09
PRICE_8B_OUTPUT = 6.0e-08

MODELS = [
    {
        "modelName": "qwen3-coder-30b-local",
        "matchPattern": "(?i)^(qwen3-coder.*)$",
        "inputPrice": PRICE_30B_INPUT,
        "outputPrice": PRICE_30B_OUTPUT,
    },
    {
        "modelName": "muse-glimmer-local",
        "matchPattern": "(?i)^(muse-glimmer.*)$",
        "inputPrice": PRICE_30B_INPUT,
        "outputPrice": PRICE_30B_OUTPUT,
    },
    {
        "modelName": "llama3-local",
        "matchPattern": "(?i)^(llama3.*)$",
        "inputPrice": PRICE_8B_INPUT,
        "outputPrice": PRICE_8B_OUTPUT,
    },
    {
        "modelName": "qwen2.5-coder-local",
        "matchPattern": "(?i)^(qwen2\\.5-coder.*)$",
        "inputPrice": PRICE_8B_INPUT,
        "outputPrice": PRICE_8B_OUTPUT,
    },
]


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--host", default=os.environ.get("LANGFUSE_HOST", "http://localhost:3002"))
    parser.add_argument(
        "--public-key", default=os.environ.get("LANGFUSE_PUBLIC_KEY", "pk-lf-jobshout-local")
    )
    parser.add_argument(
        "--secret-key", default=os.environ.get("LANGFUSE_SECRET_KEY", "sk-lf-jobshout-local")
    )
    args = parser.parse_args()

    base = args.host.rstrip("/") + "/api/public/models"
    auth = "Basic " + base64.b64encode(f"{args.public_key}:{args.secret_key}".encode()).decode()

    def call(method: str, url: str, body: dict | None = None) -> dict:
        req = urllib.request.Request(
            url,
            method=method,
            data=json.dumps(body).encode() if body is not None else None,
            headers={"Authorization": auth, "Content-Type": "application/json"},
        )
        try:
            with urllib.request.urlopen(req, timeout=30) as resp:
                raw = resp.read()
                return json.loads(raw) if raw else {}
        except urllib.error.HTTPError as err:
            sys.exit(f"{method} {url} failed: HTTP {err.code}\n{err.read().decode(errors='replace')}")

    existing, page = {}, 1
    while True:
        data = call("GET", f"{base}?page={page}&limit=100")
        for m in data.get("data", []):
            if not m.get("isLangfuseManaged"):
                existing[m["modelName"]] = m["id"]
        if page >= data.get("meta", {}).get("totalPages", 1):
            break
        page += 1

    for spec in MODELS:
        if spec["modelName"] in existing:
            print(f"exists: {spec['modelName']} ({existing[spec['modelName']]})")
            continue
        created = call("POST", base, {**spec, "unit": "TOKENS"})
        print(f"created: {spec['modelName']} ({created.get('id')})")

    print("Done — prices apply to generations ingested from now on, not retroactively.")


if __name__ == "__main__":
    main()
