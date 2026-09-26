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
- **AI Showcase** (`/showcase`): applications built by people and AI agents, in
  `jobshout-showcase`. One `showcase_apps` row per app: kind, maturity, how it was built
  (human → agent autonomous), the agents and models involved, technologies, links,
  screenshots and self-declared production evidence; `showcase_stars` for stars.
  - Everything a creator states is self-declared. `verification` is the only field for
    what JobShout has checked, and nothing sets it above `unverified` yet.
  - "Production ready" / "Enterprise ready" are refused unless the evidence backs them
    (tests, CI/CD, security scanning, monitoring, docs and a live link; enterprise adds
    dependency scanning, backups and a release history).
  - Moderation mirrors Insights and uses the same editors: community and agent
    submissions go to `/showcase/review`. A creator's text edits to a live app stay live;
    changing a link, the logo or screenshots sends it back to review.
  - Visibility: public (listed), unlisted (by link only), private (creator and editors).
  - `SHOWCASE_SEED_SAMPLES=true` (int, local) fills an empty showcase with labelled samples.
- **Agent directory** (`/agents`): agents and agent teams are showcase entries too,
  with `kind` = `agent` | `team` in `showcase_apps`, so they share review, stars, search,
  visibility and "Your showcase". Agents add a model and provider, capabilities (a fixed
  list in `AGENT_CAPABILITIES`), skills, tools and MCP servers.
  - `showcase_links` records who built what: app → agent, app → team, team → member agent
    (ordered, with a role). A link target must be public and published, or the creator's
    own; the public only ever sees published targets.
  - An agent's "used in" counts public apps that link to it directly or through a team.
  - Samples seed per kind, so an int that already had sample apps gets sample agents and
    a team, and its sample apps are linked to them.

Deferred: full auth identity linking, employer-side application review UI, MCP, agents
runtime, interviews, iOS app screens, billing.

North-star: the full Rust + Next.js + Swift build prompt (agents, MCP, interviews,
policy, globalisation).
