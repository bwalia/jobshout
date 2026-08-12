# AI PR Reviewer (PoC)

Reviews GitHub pull requests using a local `qwen3-coder:30b` via Ollama, with
OpenCode as the repository exploration engine. Nothing leaves the machine except
the review posted to GitHub.

The point of this PoC: the reviewer does **not** review the diff in isolation. It
explores the surrounding repository first — reading callers, imports, and tests —
the way a senior engineer would, and catches bugs that are invisible in the diff alone.

## Setup

```bash
python3 -m venv .venv && .venv/bin/pip install -r requirements.txt
cp .env.example .env      # then fill it in
```

```ini
GITHUB_TOKEN=            # `repo` scope — reuse the gh CLI's:  gh auth token
LOCAL_REPO_PATH=         # absolute path to the local clone to review
OLLAMA_HOST=http://localhost:11434
MODEL=qwen3-coder:30b
```

The GitHub repo is derived from that clone's `origin` remote, so it can't drift
out of sync with the code being reviewed.

## Usage

```bash
.venv/bin/python -m app.main prime                 # build the cached repo map (one-time)
.venv/bin/python -m app.main review 42 --dry-run   # print findings, post nothing
.venv/bin/python -m app.main review 42             # post the review to GitHub
.venv/bin/python -m app.main watch                 # auto-review PRs when a reviewer is requested
.venv/bin/python -m app.main watch --once          # one polling pass, then exit (safe to test)
```

### Watch mode (automatic reviews)

`watch` polls the repo every 60s (`--interval` to change). When a PR has a reviewer
requested, it:

1. posts an immediate **"review started"** comment,
2. runs the review,
3. posts the review (summary + inline comments).

Deduplication is a hidden marker in the started comment, keyed by the PR's head commit
— so each commit is reviewed once, it survives restarts with no local state, and a new
push triggers a fresh review. A failed review posts an error note and does not retry the
same commit (no retry storms); push a new commit to retry.

It reviews on **any** reviewer request, per the design. `watch` runs in the foreground;
for an always-on service on the Mac Studio, run it under `launchd` or `nohup` so it
survives logout.

Use `--dry-run` first on any new repo — it exercises the full pipeline and prints
the exact review without touching GitHub. `prime` is optional: the first review
builds the map automatically if it is missing.

### `/custom-review` from your editor (remote MCP)

An always-on MCP server (`app/mcp_server.py`, `com.review-bot.mcp` under launchd)
exposes reviews to Claude Code and Cursor on **any** machine — no Python, Ollama, or
review-bot checkout needed there. It serves a start/poll job API (`start_review`,
`review_status`, `prime`, `list_repos`): every tool answers in under a second, and the
editor's agent polls until the review finishes, so no client tool-timeout is ever hit.
One worker thread serializes all reviews (a shared clone cannot host two concurrent
worktree-pruning reviews); extra requests queue with a visible position. Repos it may
review are allowlisted in `repos.json`; clones are managed on demand under
`~/.cache/review-bot/clones`.

Developer setup (~2 minutes): see **[docs/client-setup.md](docs/client-setup.md)**.
Design record: ADR-0002 in the software-design-patterns repo.

### Repo map (opt-in)

`prime` builds a compact **repo map** — a distilled note of the architecture,
conventions, and gotchas — cached **outside** the repo. When present and fresh, it is
injected into each review as orientation. It is **opt-in**: reviews consume a cached map
but never build one, so a review never silently triggers a multi-minute prime. If the
map has gone stale (HEAD drifted past `MAP_REFRESH_COMMITS` commits) a review skips it
and tells you to `prime --force`.

Honest status: measured on a self-contained PR, the map produced **no speedup** — the
targeted-read prompt already minimises exploration, and the changed files must be read
fresh regardless (that is the point of the PR). It did not hurt review quality. It is
kept opt-in because it *may* help large PRs that touch many modules, where summarised
cross-module context replaces real exploration — but that benefit is unmeasured. Left
off, it costs nothing.

Note: `--continue`/session replay is deliberately **not** used. Replaying a prior
transcript re-feeds every past file read into context, which the model must re-process —
that grows cost per review, the opposite of the goal. A small distilled map is the only
form of "memory" that could help; a full transcript is a trap.

## How it works

```
review <PR#>
  → fetch PR metadata, changed files, patches           (github/client.py)
  → parse @@ hunks → set of commentable lines           (github/diff.py)
  → git fetch + worktree at PR head SHA                 (reviewer/workspace.py)
  → opencode run --auto -m ollama/<model> --dir <wt>    (opencode/runner.py)
       agent explores the repo with its own tools,
       then returns structured JSON
  → validate findings against commentable lines         (reviewer/schema.py)
  → POST one review: inline comments + summary          (github/review.py)
  → remove worktree
```

### OpenCode is the LLM caller

OpenCode already drives qwen3-coder through Ollama, and its built-in tools
(read/grep/glob/bash) already do repository navigation. So exploration and review
happen in **one** agent run rather than two hops (explore → re-prompt a separate
LLM). The second hop would re-serialize context into a fresh prompt and throw away
everything the agent learned while exploring.

This is why there is no separate `llm/` module: `OLLAMA_HOST` and `MODEL` become the
OpenCode provider definition in `opencode/provider.py`. The `.env` contract is unchanged.

OpenCode ships no Ollama provider by default, so we generate one against Ollama's
OpenAI-compatible endpoint (`/v1`) and inject it via `OPENCODE_CONFIG` — your own
OpenCode config is never touched.

### A read-only reviewer agent, not the default one

We register our own `reviewer` agent instead of using OpenCode's default `build`
agent, which enables every tool. Two of those tools are actively harmful here:

- **`task`** spawns nested sub-agents. Each is a full agent loop against the same
  local model, so a review fans out into several concurrent model runs and stops
  converging. This was a real failure, not a theoretical one: an early run against
  a 4-file PR spawned three sub-agents and blew past a 15-minute timeout. With
  `task` disabled the same review finishes in minutes.
- **`write`/`edit`** would let the reviewer modify the code it is judging. A reviewer
  must never do that.

`todowrite`/`todoread` are planning scaffolding a single-shot review doesn't need.
The allowed set is `read`, `grep`, `glob`, `list`, `bash` — enough to explore, and
nothing more. See `AGENT_TOOLS` in `opencode/provider.py`.

### Merge or fix, up front

Every review opens with one of two headlines, so the answer is the first thing read:

- **🛑 FIX** — the change breaks something, will not actually do what the PR promises,
  or carries a high-severity problem.
- **✅ MERGE** — nothing blocking. Smaller notes still get raised, but they do not
  hold up a merge.

A *medium* "this will not fix the reported bug" blocks; a *medium* readability note
does not. See `Review.blocking` in `reviewer/schema.py`.

### Two questions first, in plain language

The reviewer answers two questions before anything else, because they are what a
human reviewer actually asks:

1. **Does it break anything** that works today?
2. **Will it actually work** — does the change genuinely deliver the fix or feature
   the PR promises? A fix that misses the real case has not done its job.

Findings are tagged `breaks`, `intent`, or `other`, and the first two always sort
above the rest — a *medium* `intent` finding outranks a *high* `other` finding.

GitHub renders inline comments in file/line order, so we cannot force a critical
comment to appear first in the diff view. The summary is the only place ordering is
ours to control, so it leads with the verdict and a "Read these first" section.

Comments are written for a human: plain English, no jargon, saying what goes wrong
and when ("if someone uploads a statement with no dates in the header, this crashes")
rather than naming it abstractly.

### Context is pulled, not pushed

We send the diff, not the repository. The agent then pulls exactly the context it
needs via its own tools. Only what it actually reads enters the model's context.

### Worktrees, not clones

Each review checks out the PR head in a `git worktree` off your existing local repo.
It reuses the local object store (no per-request clone) and **never disturbs your
working directory or current branch**. The worktree is removed afterwards.

### Why line validation matters

GitHub rejects (`422`) any inline comment on a line outside the diff hunks — and one
bad line fails the *entire* review. A model asked for line numbers will eventually
name one outside the diff, so `github/diff.py` maps the commentable lines up front and
`reviewer/schema.py` validates every finding against it. Findings that can't be anchored
are **kept and moved into the summary** rather than dropped or allowed to fail the post.

## Layout

```
app/
  config/     .env loading and validation
  github/     client.py (REST), diff.py (hunk→line map), review.py (payload),
              markers.py (head-sha dedup marker, shared by watcher and jobs)
  opencode/   provider.py (Ollama config), runner.py (subprocess + JSON recovery)
  reviewer/   agent.py (orchestration), schema.py (validation), workspace.py (worktree)
  prompts/    review.py — the reviewer prompt
  repos.py    repo allowlist (repos.json) + managed clones
  jobs.py     single-worker job queue (the global review lock)
  mcp_server.py  Streamable HTTP MCP server (start/poll job API, bearer auth)
  main.py     CLI
tests/        pytest suite (markers, repos, jobs, MCP tool layer)
```

## Scope

PoC grown one deliberate step: one reviewer agent, no database, and a single-worker
in-memory job queue serving the MCP server. PR detection is the watcher or the editor
command; `review_pull_request()` still takes a PR number and nothing else — the reason
both the watcher and the MCP job runner can wrap it unchanged.
