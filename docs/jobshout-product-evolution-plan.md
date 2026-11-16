# JobShout.com product evolution plan

Companion to the JobShout.com implementation prompt. Phases ship behind
feature flags (dark on int first unless noted). AI drafts only; humans confirm
spend / send / hire / publish.

## Shipped on `master` (merged PRs)

| Phase | PR | Flag(s) | Migrations |
| --- | --- | --- | --- |
| 0 Foundation | [#352](https://github.com/bwalia/jobshout/pull/352) | `EMAIL_OUTBOX_ENABLED` | `0012_accounts_events` |
| 1 Members + Dashboard | [#353](https://github.com/bwalia/jobshout/pull/353) | `MEMBERS_RICH_ENABLED`, `DASHBOARD_ENABLED`, `MESSAGING_ENABLED` | `0013_members_dashboard` |
| 2a Projects live | [#354](https://github.com/bwalia/jobshout/pull/354) | `PROJECTS_LIVE` | `0014_projects_live` |
| 2b Credits | [#356](https://github.com/bwalia/jobshout/pull/356) | `CREDITS_ENABLED`, `PURCHASES_ENABLED` | `0015_credits` |
| 5a First AI | [#357](https://github.com/bwalia/jobshout/pull/357) | `AI_FEATURES` | `0016_ai_drafts` |
| 3 Linked Hire | [#358](https://github.com/bwalia/jobshout/pull/358) | `HIRE_ENABLED` | `0017_linked_hire` |
| 4 Linked Groups | [#359](https://github.com/bwalia/jobshout/pull/359), [#366](https://github.com/bwalia/jobshout/pull/366) | `GROUPS_ENABLED` (on int) | `0018_linked_groups` |
| 5b Rest of AI | [#360](https://github.com/bwalia/jobshout/pull/360) | `AI_FEATURES` | — |
| 6 Teams / analytics | [#361](https://github.com/bwalia/jobshout/pull/361) | `TEAMS_ENABLED` | `0019_teams_agencies` |
| Jobs/auth hardening | [#364](https://github.com/bwalia/jobshout/pull/364) | — | `0020_saved_jobs_alerts` |

Follow-ups: [#365](https://github.com/bwalia/jobshout/pull/365) local/dev auth, [#366](https://github.com/bwalia/jobshout/pull/366) groups posts UI.

## How to try on int

1. Deploy/promote jobshout-com as usual (Ring Promoter). Flags stay dark except
   `groups.enabled: true` on int for testing.
2. Flip a flag in `deploy/helm/jobshout-com/values-int.yaml` (or `--set`), then
   restart `jobshout-com-api` with an explicit deployments list.
3. Sign in on https://int.jobshout.com — never use `?email=` for identity.

Local: `cd jobshout-com && cargo test --workspace && make lint`;
`cd jobshout-com/web/nextjs && npm run typecheck && npm run lint`.

## Intentional leftovers

- Escrow / milestone payouts / client fund holding (explicitly out of scope).
- Full Stripe Checkout UI wiring against live keys (webhook grant path exists;
  `PURCHASES_ENABLED` stays off).
- Interview scheduling ICS + scorecards UI (schema/pipeline stages exist; product UI thin).
- `/following` still primarily showcase/Insights-derived; domain_events enrichment partial.
- AI Agent Marketplace hire-on-project flow beyond showcase linking.
- Production password / prod deploy never part of this workstream.

Gap-fill (nav, hire console, `/opportunities`, mobile bottom bar, Interview Prep agent)
ships as a follow-up PR on top of the phase series above.
