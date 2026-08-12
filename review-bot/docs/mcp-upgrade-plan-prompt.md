# Plan request: /custom-review — remote MCP upgrade for review-bot

> **Superseded** by [plans/01-mcp-remote-upgrade.md](plans/01-mcp-remote-upgrade.md),
> §"Design change forced by Phase 0 research": the single long `review_pr()` call this
> prompt assumed was falsified for Cursor (fixed ~60s tool timeout, progress does not
> extend it) — the shipped design is a start/poll job API (`start_review` +
> `review_status`). Executed 2026-08-12; recorded as ADR-0002 in
> software-design-patterns.

You are planning an upgrade to **review-bot**, an existing, working local-first AI PR
reviewer. Produce a phased implementation plan. Do not write implementation code yet.

## Goal

A developer on **any computer**, in **Cursor or Claude Code**, finishes work on a PR and
types `/custom-review <pr-number>`. The review runs on the central **Mac Studio** (where
review-bot, OpenCode, Ollama/qwen3-coder:30b, and all secrets live) against the repo the
developer currently has open, and the review is posted to GitHub. The developer's machine
installs **nothing** — enabling the feature is two small editor config entries: the MCP
server URL and the `/custom-review` command file.

## Read these first (in this order)

1. `README.md` — the whole existing system: pipeline, watch loop, dedup markers, design
   rationale.
2. `app/reviewer/agent.py` — `review_pull_request()`, the function the MCP server wraps.
   Today it takes only a PR number; the repo comes from `.env`.
3. `app/config/settings.py` — the single-repo `.env` contract (`LOCAL_REPO_PATH`) that
   must be generalized for multi-repo.
4. `app/github/client.py` — note `repo_slug()` already derives `owner/repo` from a
   clone's `origin` remote.
5. `app/watch.py` — the `started:{head_sha}` marker dedup that manual reviews must
   participate in.
6. ADR baseline: `~/projects/software-design-patterns/adrs/review-bot/adr-0001-mcp-server-with-command-files.md`.
   **Caution:** ADR-0001 assumed a per-machine stdio server. The decisions below
   supersede its transport and install model; the plan must include writing ADR-0002
   recording the change.

## Decisions already made — do not relitigate

1. **Central runner.** One MCP server process on the Mac Studio, run as a launchd
   service (pattern: the existing `com.review-bot.watch` LaunchAgent). Remote-capable
   MCP transport (Streamable HTTP). Developers' machines run no part of the pipeline.
2. **Client enablement is config-only.** Claude Code: register the server by URL
   (`claude mcp add --transport http ...`) plus a user-level
   `~/.claude/commands/custom-review.md`. Cursor: `mcp.json` URL entry plus its
   command file equivalent. The plan delivers exact copy-paste onboarding steps for both
   editors, written for a developer who knows nothing about MCP.
3. **Multi-repo via the tool signature.** Tool is
   `review_pr(repo: "owner/name", pr_number, dry_run=false)`. The command file instructs
   the editor's agent to fill `repo` from the current workspace's `origin` remote. The
   Mac Studio maintains its own managed clones (auto-clone on first request into a cache
   directory, `git fetch` before each review, an explicit allowlist of reviewable repos).
4. **Default posts to GitHub.** `/custom-review 42` posts; `/custom-review 42 dry-run`
   runs the full pipeline and returns findings to the editor without posting.
5. **One review at a time.** Server-side queue/lock across editor sessions and the
   watcher, so two 30B model runs never overlap. Queue position and pipeline stages are
   reported to the client as MCP progress notifications (map the existing `on_progress`
   callback; reviews take 2–5 minutes and progress is what keeps the client's tool call
   alive). If a client proves progress-deaf, fall back to a `start_review` /
   `review_status` fire-then-poll tool pair.
6. **Dedup with the watcher.** Manual reviews post the same hidden
   `started:{head_sha}` marker comment, so the watch loop skips commits already reviewed
   manually. No database.
7. **No stopgap.** Skip the Claude-Code-only shell-out interim version; build the MCP
   server directly.
8. **Pipeline core unchanged.** The OpenCode read-only agent, prompts, schema
   validation, and worktree mechanics stay as they are. The only permitted refactor is
   generalizing settings/orchestration from "one repo from `.env`" to "repo passed per
   request".

## Constraints

- Secrets (`GITHUB_TOKEN`, Ollama host, clone paths) never leave the Mac Studio; clients
  only send `repo` + `pr_number` and receive `{verdict, findings[], url}`.
- The server must require authentication — shared bearer token minimum. Assume the
  Mac Studio is reachable over LAN or Tailscale; the plan states the chosen reachability
  approach and how a new developer gets the URL + token.
- Only allowlisted repos may be reviewed; the bot's GitHub token scope bounds what it can
  post to. Reject unknown repos with a clear error the editor agent can relay.
- The watcher and the MCP server share one Ollama and one clone cache — the lock in
  decision 5 covers both.

## The plan must deliver

- Phased implementation where every phase ends in something runnable and testable.
- File-level change list in review-bot (new modules, modified modules, launchd plist,
  requirements change).
- Settings refactor design for multi-repo (registry/allowlist format, clone cache
  location, migration for the existing single-repo `.env`).
- MCP server design: transport, tool schemas, auth, progress forwarding, queue/lock,
  error surfaces (unknown repo, PR not found, pipeline failure, queue timeout).
- Clone manager design: where clones live, fetch strategy, interaction with the
  existing worktree isolation.
- Client onboarding doc: exact steps + the two command files, for Claude Code and Cursor.
- Test plan: unit coverage for new modules, plus an end-to-end proof — a dry-run and a
  posted review triggered from a second machine, in both editors, against a real PR.
- ADR-0002 draft for `~/projects/software-design-patterns/adrs/review-bot/` recording
  the central-runner + HTTP transport + config-only client decisions (supersedes parts
  of ADR-0001).
- Risks and open questions, each with a recommended default.

## Acceptance criteria

- From a different computer than the Mac Studio, in both Cursor and Claude Code:
  `/custom-review <n>` on an open, allowlisted repo posts a review to GitHub, with
  visible progress and no client timeout.
- `dry-run` returns findings into the editor and posts nothing.
- Two simultaneous requests: the second queues (visible via progress) and both complete.
- The watcher does not re-review a commit that was manually reviewed.
- The developer machine needs no Ollama, Python, or review-bot checkout — verified by
  onboarding a machine that has none of them.
