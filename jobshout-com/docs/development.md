# JobShout.com — development

## Prerequisites

- Rust stable (1.90+)
- Node 20+
- Docker

## Quick start

```bash
cd jobshout-com
make docker-up
make migrate
make seed
make api    # terminal 1 — http://127.0.0.1:8088/health
make web    # terminal 2 — http://127.0.0.1:3010
```

```bash
curl -s http://127.0.0.1:8088/api/v1/jobs | jq '.data | length'
```

## Candidate profile + matching

1. Open http://127.0.0.1:3010/profile and save skills / preferred roles.
2. You will be redirected to ranked matches.
3. Agents can load:

```bash
PROFILE_ID=…
curl -s "http://127.0.0.1:8088/api/v1/profiles/$PROFILE_ID/matching-context" | jq .
```

## Posting and applying

Post a role at http://127.0.0.1:3010/post-job — the form previews the board card live,
and publishes through `POST /api/v1/jobs`. Apply from any job at `/jobs/{id}/apply`.

```bash
JOB_ID=$(curl -s http://127.0.0.1:8088/api/v1/jobs | jq -r '.data[0].id')

curl -s -X POST "http://127.0.0.1:8088/api/v1/jobs/$JOB_ID/applications" \
  -H 'content-type: application/json' \
  -H "x-jobshout-user-email: ada@example.com" \
  -H "x-jobshout-user-name: Ada" \
  -H "x-jobshout-internal-token: $INTERNAL_TOKEN" \
  -d '{"full_name":"Ada Lovelace","email":"ada@example.com","cover_letter":"…"}' | jq .

# Everything one candidate has applied to, with the job attached
curl -s -H "x-jobshout-user-email: ada@example.com" \
  -H "x-jobshout-user-name: Ada" \
  -H "x-jobshout-internal-token: $INTERNAL_TOKEN" \
  "http://127.0.0.1:8088/api/v1/applications" | jq '.data[].job.title'
```

Track them in the UI at http://127.0.0.1:3010/applications (sign-in required).

## Theming

The web app ships light and dark themes. Tokens live in `app/globals.css` as RGB
triplets (`--brand`, `--ink`, …) re-declared under `.dark`; Tailwind maps them in
`tailwind.config.js`. An inline script in `app/layout.tsx` resolves the theme before
first paint, so there is no flash. Add a colour by declaring it in **both** blocks.

## Sign-in (local + OAuth)

Copy `web/nextjs/.env.example` to `web/nextjs/.env.local`, set `NEXTAUTH_SECRET`, then
restart `make web`. Sign-in UI lives at `/login`.

### Local / dev login (no OAuth secrets)

Prior demos skipped login because Google/Facebook/Apple/LinkedIn **client secrets were
not in the environment** — the login page correctly disabled every social button.
For local testing without IdP apps:

```bash
# web/nextjs/.env.local
AUTH_DEV_LOGIN=true
APP_ENV=local
NEXTAUTH_SECRET=any-long-random-string
NEXTAUTH_URL=http://127.0.0.1:3010
```

Open http://127.0.0.1:3010/login and use **Sign in as local user** (default
`ada@example.com`). The credentials provider is refused when `APP_ENV` is
`int` / `test` / `acc` / `prod` even if the flag is set. Never enable
`AUTH_DEV_LOGIN` in Helm or Vault for a ring.

API calls from the web still forward the session as `x-jobshout-user-*` headers
plus `x-jobshout-internal-token` (same as OAuth sessions).

### Real OAuth (optional locally; required for int social buttons)

Set any pair below in `.env.local` (local) or in WSLVault (rings). Do **not**
paste secrets into chat, tickets, or git.

| Env keys | Notes |
| --- | --- |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | Google Cloud OAuth client; redirect `https://<host>/api/auth/callback/google` |
| `FACEBOOK_CLIENT_ID` / `FACEBOOK_CLIENT_SECRET` | Meta app |
| `APPLE_ID` / `APPLE_SECRET` | Apple Sign In |
| `LINKEDIN_CLIENT_ID` / `LINKEDIN_CLIENT_SECRET` | LinkedIn OAuth app |

**Vault (k3s1 / Ring Promoter):** create KV path
`jobshout-com/<ring>/oauth` with the env key names above (exact names). Materialise
into K8s Secret `jobshout-com-oauth`; the web Deployment mounts it via optional
`envFrom` (see Helm `web.yaml`). `NEXTAUTH_SECRET` stays in Helm-managed
`jobshout-com-auth` (auto-generated when empty).

Platform JobShout (panel) Google/LinkedIn paths under `jobshout/<ring>/google` and
`jobshout/<ring>/linkedin` are a **different** app — do not reuse those client IDs
unless the OAuth consent screen redirect URIs also list jobshout.com callbacks.

### Linked Groups (Phase 4)

Schema + `POST /api/v1/groups` (create) and list/search exist behind
`GROUPS_ENABLED` (Helm `groups.enabled`, dark on int). Tables for posts /
comments / reactions are migrated, but HTTP routes and a rich `/groups` UI for
LinkedIn-like feeds are still thin — turning the flag on lets you create/list
groups once signed in; full post/comment UX is not complete yet.

## Deploy (int → Ring Promoter)

Public int host: **https://int.jobshout.com**

```bash
# Once: Cloudflare CNAME + wslproxy vhost
gh workflow run register-edge-vhost.yml --repo bwalia/jobshout \
  -f host=int.jobshout.com \
  -f zone=jobshout.com \
  -f server_spec=jobshout-com/deploy/edge/wslproxy-server-int.json \
  -f health_path=/health

# Or from deploy-jobshout-com.yml with REGISTER_EDGE=true
```

CI: `.github/workflows/deploy-jobshout-com.yml` builds `jobshout-com/{api,web}:jsc-v…`
and seeds https://rp.workstation.co.uk/?app=jobshout-com

## Ports

| Service | Port |
| --- | --- |
| API | 8088 |
| Web | 3010 |
| Postgres | 5434 |
| Redis | 6380 |
| NATS | 4223 |
| MinIO | 9010 / 9011 |

Chosen to avoid clashing with the platform stack (API 8080, UI 3001, Postgres 5432).

## Tests

```bash
make test
make lint
```
