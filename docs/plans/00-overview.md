# JobShout — Agent, Task Manager, and Chatbot Improvement Plan

Work is sequenced: **evals first** (Gmail, Research, Article baseline), then **Task Manager launch/board**, then **chat routes through the same launcher**. Article image upgrades and Gmail URL-follow are code changes after their baselines.

## How things work today

```
Chat ──► platformtools (research_run, article_generate, mail_sync, agent_execute)
              │
              ▼
         specialist APIs     (usually no board task)

Task Manager ──► POST /tasks ──► launch.ts ──► same specialist APIs
                      │
                      ▼
                 tasks table     (card exists; article/mail/research often unlinked)
```

Chat does **not** go through Task Manager. `runAgentExecute` in `server/internal/platformtools/execute.go` dispatches specialists directly. `web/nextjs/lib/agents/launch.ts` creates a board task first, then hits the same APIs — but article/mail/research often do not attach `task_id`, so the board card and the run drift apart.

## Documents

| Doc | Surface | First move |
|-----|---------|------------|
| [01-gmail-agent.md](01-gmail-agent.md) | Mail / Gmail | Offline dummy-email evals, then extract inbound URLs |
| [02-article-agent.md](02-article-agent.md) | Article Writer | Cover/inline baseline evals, then body images + unique covers |
| [03-research-agent.md](03-research-agent.md) | Research Agent | Small confirmation evals; freeze unless they fail |
| [04-task-manager.md](04-task-manager.md) | Task Manager | Server `tasklaunch` + per-agent fields + board stay |
| [05-chatbot.md](05-chatbot.md) | Chat | Scripted invocation evals, then route through `tasklaunch` |
| [06-task-history-and-board-ux.md](06-task-history-and-board-ux.md) | Board + Task Manager UX | Run + history from the card; completed-at; honest progress |

## Implementation order

1. Write these plan docs (this folder).
2. Agent evals (Gmail dummy emails, Research confirmation, Article cover/inline baseline).
3. Chat scripted agent-invocation evals.
4. Server `tasklaunch` + Task Manager client, `launch_values` on the task, board navigation.
5. Gmail URL extract + form copy + `agentschema` mail Fields.
6. Article body images + cover brief; pass `context` + `task_id`.
7. Chat tools through `tasklaunch`; interview project if the org has 2+; tighten prompt; inject memory into context.

Research code stays frozen unless its evals fail. Article image work and Gmail URL-follow wait for their failing baseline tests so the diff is measurable.

## Chat project rule

When chat runs an agent, every execution creates a Task Manager board task:

- If the org has exactly one project, use it.
- If the org has more than one, interview `project` (name chip list) unless the session `last_project` matches a clear “that project” reference.

## Model

Do not swap `CHAT_MODEL` on gut feel. Default remains `qwen3-coder:30b`. A/B `qwen3:30b-a3b` only if live evals still fail on intent after launcher + prompt.
