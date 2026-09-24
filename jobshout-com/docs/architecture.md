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
- **Insights** (`/insights`): the news, articles, blogs, podcasts and videos hub, in
  `jobshout-content`. One `insights_items` table with a `kind` discriminator
  (post | article | blog | podcast | video), topics, full-text search, and a review
  queue: community submissions land in `pending_review`, editors approve, reject with a
  note, feature or archive. Markdown is rendered and sanitised server-side (ammonia);
  media embeds are built only from allowlisted hosts (YouTube, Vimeo, Spotify, Apple
  Podcasts) or direct audio/video files. RSS at `/insights/rss.xml`, an iTunes-tagged
  podcast feed at `/insights/podcast.xml` (file-hosted episodes only), a weekly digest
  archive at `/newsletter`, and double opt-in newsletter signup.
  - Editors are `INSIGHTS_STAFF_EMAILS` on the API — a stopgap until real roles exist.
  - Identity: the web tier forwards the signed-in user as `x-jobshout-user-*` headers,
    signed with `JOBSHOUT_INTERNAL_TOKEN` (chart-generated `jobshout-com-internal`
    Secret). The gateway exposes `/api/` publicly, so the API rejects unsigned identity
    headers and nginx strips them from public traffic. The API refuses to start without
    the token unless `JOBSHOUT_ALLOW_UNSIGNED_IDENTITY=1`.
  - No mailer yet: newsletter confirmation links are logged by the API.
  - `INSIGHTS_SEED_SAMPLES=true` (int, local) fills an empty hub with labelled samples.

Deferred: full auth identity linking, employer-side application review UI, MCP, agents
runtime, interviews, iOS app screens, billing.

North-star: the full Rust + Next.js + Swift build prompt (agents, MCP, interviews,
policy, globalisation).
