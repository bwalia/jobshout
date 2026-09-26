# Gmail / Mail Agent — Plan

## Purpose

Monitor incoming organisation Gmail, classify threads, optionally commission the Research Agent, draft a reply, and wait for a human to Approve before anything is sent.

Example: a client emails “What is the price of this machine?” and includes a product URL. The agent should research that link (and any playbook pages), then draft a reply from the brief.

## Current behaviour

Pipeline in `server/internal/service/mail_service.go` `processThread`:

1. Classify (`Classifier` / `HeuristicClassify`).
2. Optional research.
3. Draft (`Drafter` / `HeuristicDraft`).
4. Human Approve → `ApproveSend` only.

Research runs when:

- Playbook `watch_knowledge_urls` is set (pinned fetch, no web search), or
- Classifier sets `needs_research=true` (open-web search on subject + body).

**Gap:** links inside an inbound email are **not extracted**. A client mail with `https://vendor.com/machine-x` will not research that URL unless it was pre-pinned or open-web search happens to find it.

Task Manager launch (`web/nextjs/lib/agents/launch.ts` `case "mail"`): PATCH playbook + POST `/mail/sync`. Chat tools: `mail_sync` / `mail_list_drafts` only. `server/internal/agentschema/schema.go` mail schema has **zero Fields**.

## Evals first (baseline)

File: `server/internal/service/mail_eval_test.go`

Use the existing `fakeGmail` + `scriptClass` + `fakeResearch` pattern from `mail_service_test.go`. No real Gmail. No inject API.

Dummy emails:

| ID | Scenario | Assert |
|----|----------|--------|
| M1 | Price + link in body | Research commissioned; `Request.URLs` includes the body URL |
| M2 | Availability + link | Same shape |
| M3 | Pinned playbook only, no body URL | Pinned research runs even if `needs_research=false` |
| M4 | Newsletter / ignore | No research, no draft |
| M5 | Draft quality | Reply must not claim the email was sent; must use research brief when present |

M1/M2 fail until inbound URL extract lands. M3–M5 should pass on current code (M3 already has a sibling in `mail_service_test.go`).

## Code changes after evals

1. **Extract http(s) URLs** from inbound subject/body (`server/internal/mail/knowledge.go`). Merge with playbook URLs → `research.Request.URLs`. Cap (5). Skip unsubscribe/tracking hosts.
2. **Classifier:** if a URL is present, prefer `needs_research=true` unless action is ignore (`HeuristicClassify` + prompt hint).
3. **Task Manager copy** in `web/nextjs/lib/agents/input-schemas.ts`: group “Who to watch” vs “How to answer”. Empty watch = last 7 days unread. Knowledge links are optional extras, not the only pages researched.
4. **Link the board task:** persist `task_id` / `launch_values` on the mail launch. After draft-ready, update task description with draft subject + thread link.
5. **Chat schema parity:** add mail Fields to `agentschema`. Chat still goes through `tasklaunch`, not a raw `mail_sync`.

## Acceptance

- Eval suite green for ignore/pinned; price+link eval passes after URL extract.
- Create & run Mail from Task Manager still saves playbook and queues sync; Gmail-disconnected still returns the connect toast (409/503).
- Nothing is sent without Approve.
