# JobShout.com architecture (Phase 1)

JobShout.com is the **AI-native employment marketplace** packaged as
`jobshout-com/` inside the JobShout ecosystem monorepo. It is separate from the
existing Go agent platform (`server/` + `web/nextjs/`).

## Products

| Path | Product |
| --- | --- |
| `server/`, `web/nextjs/` | JobShout agent orchestration platform |
| `jobshout-com/` | JobShout.com marketplace (Rust + Next.js + future Swift) |

## Phase 1 shape

```text
Next.js (:3010)  →  Axum API (:8088)  →  Postgres (:5434)
```

Deployed int ring: **https://int.jobshout.com** via Helm + Ring Promoter
(`rp.workstation.co.uk/?app=jobshout-com`). See [docs/deploy.md](deploy.md).

Implemented:

- Cargo workspace with domain crates (many stubs) matching the north-star layout
- `jobshout-jobs` + `GET/POST /api/v1/jobs`
- Candidate profiles + explainable matching for Career agents
  (`POST /api/v1/profiles`, `GET …/matches`, `GET …/matching-context`)
- Marketplace web: home, job board, profile builder, ranked matches, social login scaffolding
- Own `docker-compose.yml` and Helm chart under `deploy/`

Deferred: full auth identity linking, MCP, agents runtime, interviews, iOS app screens, billing.

North-star: the full Rust + Next.js + Swift build prompt (agents, MCP, interviews,
policy, globalisation).
