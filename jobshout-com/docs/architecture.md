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
  (`POST /api/v1/profiles`, `GET /api/v1/profiles/me`, `GET …/matches`,
  `GET …/matching-context`). **Signed identity required**; owner-only (or trusted
  matching agent for matching-context). Email / CV / salary never leave the owner path.
- `jobshout-applications` + apply flow
  (`POST/GET /api/v1/jobs/{id}/applications`, `GET …/applications/check`,
  `GET /api/v1/applications`). Signed identity required; a job's applications are
  visible only to its poster (or staff). Re-applying updates the existing row.
- Accounts + roles (`accounts`, `account_identities`, `account_roles`) with
  `jobshout-auth` actor extraction and `jobshout-policy::can()`. Staff role is
  bootstrapped from `INSIGHTS_STAFF_EMAILS` (env kept).
- Event spine (`domain_events`, `notifications`, `notification_preferences`,
  `email_outbox`) + worker mailer / scheduler (Helm-gated; `EMAIL_OUTBOX_ENABLED`).
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
  - Editors hold the `staff` account role (bootstrapped from `INSIGHTS_STAFF_EMAILS`).
  - Identity: the web tier forwards the signed-in user as `x-jobshout-user-*` headers,
    signed with `JOBSHOUT_INTERNAL_TOKEN` (chart-generated `jobshout-com-internal`
    Secret). The gateway exposes `/api/` publicly, so the API rejects unsigned identity
    headers and nginx strips them from public traffic. The API refuses to start without
    the token unless `JOBSHOUT_ALLOW_UNSIGNED_IDENTITY=1`.
  - Newsletter confirmations enqueue to `email_outbox` when `EMAIL_OUTBOX_ENABLED`
    is on; otherwise the confirm URL is logged (as before).
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
  list in `AGENT_CAPABILITIES`), tools and MCP servers, and **skills with proficiency**
  (`showcase_skills` catalog + `showcase_agent_skills` levels 1–10). Browse at
  `/agents/skills` and `/agents/skills/{slug}`; agent profiles show vision-style bars.
  Creators can add new skill names on submit (upsert into the catalog).
  - `showcase_links` records who built what: app → agent, app → team, team → member agent
    (ordered, with a role). A link target must be public and published, or the creator's
    own; the public only ever sees published targets.
  - An agent's "used in" counts public apps that link to it directly or through a team.
  - Samples seed per kind, so an int that already had sample apps gets sample agents and
    a team, and its sample apps are linked to them.
- **Hiring** (showcase ↔ jobs): any showcase entry can link open jobs from the board
  (`showcase_job_links`). Jobs now record `poster_email` when posted by a signed-in
  user — never returned by the jobs API — and a creator may link only open jobs they
  posted (editors: any open job; a job already on an entry may stay). Closed jobs drop
  off entries without an edit. Entry pages show "Open roles", job pages show "What
  you'd work on", and `collection=hiring` lists entries with at least one open role.

- **Members and following** (`/members`, `/members/{handle}`, `/following`), in
  `jobshout-users`. A `members` row is the public face of an account: a handle (from the
  sign-in name, never the email), a name, a one-line headline and a follower count. The
  email is the private join key to `creator_email` / `author_email` and is never
  returned. Members are created when someone signs in to follow or publishes, and
  backfilled at API startup for everyone who already has public work (platform agents
  such as the Article Writer included, flagged `is_agent`).
  - Follows: `member_follows` (people and platform agents) and `showcase_follows` (any
    published app, agent or team). Counts are recomputed in the follow transaction, as
    stars are. Nobody can follow themselves.
  - Feed (`GET /api/v1/following/feed`): read on demand from the showcase and Insights
    tables, 90 days back, nothing written at publish time. Followed members' new public
    entries and Insights; followed entries revised while live (`revised_at`, set only by
    a creator's edit, not by moderation or classification); and new public apps or teams
    that link a followed agent or team. A followed member's new app that also uses a
    followed agent shows once.
  - Profiles list only public, published work. Bylines on showcase, agent and Insight
    pages link to them. There are no notifications yet: the feed is pull-only.

- **Projects live (Phase 2a, flag-gated):** migration `0014` adds owner/title/status/
  visibility on projects, invitations, engagements, engagement reviews. Wired
  `jobshout-freelance` into the API with signed Actor identity (no random UUIDs).
  Web `/projects` reads the API; `/projects/post` and bid form submit for real.
  `/freelancers` redirects to `/members`. Scheduler closes expired bidding windows.
  Flag `PROJECTS_LIVE` (+ optional `PROJECTS_SEED_SAMPLES`) dark on int.

- **Dashboard & rich members (Phase 1, flag-gated):** migration `0013` adds experience,
  education, certifications, portfolio links, skills-with-evidence, connections, reviews,
  achievements, privacy settings, messaging tables, dashboard layouts and GDPR request
  rows. APIs: `GET /api/v1/dashboard`, talent search, rich profile, connect, messaging,
  block/report. Web: `/dashboard`, `/messages`, updated nav. Flags (dark on int):
  `MEMBERS_RICH_ENABLED`, `DASHBOARD_ENABLED`, `MESSAGING_ENABLED`.

- **Credits (Phase 2b, flag-gated):** migration `0015` adds append-only `credit_ledger`
  (balance = sum), packs, subscription plans, Stripe webhook idempotency, daily bid caps.
  Bid cost bands by budget; confirm dialog before spend; free 10/month when
  `CREDITS_ENABLED`; purchases/`PURCHASES_ENABLED` stay off until volume. Web `/pricing`
  and `/credits`. No escrow or payouts.

- **First AI (Phase 5a, flag-gated):** Go builtins `proposal_generator`,
  `project_breakdown`, `talent_matchmaker` (own packages, registered via
  agentmodules). jobshout.com calls platform `POST /tasks/launch` through
  `PlatformClient`; results land in `ai_drafts` for human approve/discard.
  Flag `AI_FEATURES` dark on int. No second AI runtime.

- **Linked Hire (Phase 3, flag-gated):** crate `jobshout-hire`, migration `0017`
  (orgs, members, requisitions, pipeline stages, shortlist, invitations). APIs under
  `/api/v1/hire/*` are org-scoped. Web `/hire`. Flag `HIRE_ENABLED` dark on int.
  Interviews/scorecards and recruiter analytics still thin.

Deferred: employer-side application review UI, MCP, interviews depth, iOS,
Linked Groups, remaining AI, Teams/Agencies.

North-star: the full Rust + Next.js + Swift build prompt (agents, MCP, interviews,
policy, globalisation).
