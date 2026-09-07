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
- `jobshout-applications` + apply flow
  (`POST/GET /api/v1/jobs/{id}/applications`, `GET …/applications/check`,
  `GET /api/v1/applications?email=`). Re-applying with the same email updates the
  existing application rather than creating a duplicate.
- Marketplace web: landing, job board with URL-driven search/filter/sort, job detail,
  apply, post a job (with live preview), profile builder, ranked matches, application
  tracking, social login scaffolding, light/dark theming
- Own `docker-compose.yml` and Helm chart under `deploy/`

Deferred: full auth identity linking, employer-side application review UI, MCP, agents
runtime, interviews, iOS app screens, billing.

North-star: the full Rust + Next.js + Swift build prompt (agents, MCP, interviews,
policy, globalisation).
