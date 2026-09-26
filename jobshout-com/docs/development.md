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
  -d '{"full_name":"Ada Lovelace","email":"ada@example.com","cover_letter":"…"}' | jq .

# Everything one candidate has applied to, with the job attached
curl -s "http://127.0.0.1:8088/api/v1/applications?email=ada@example.com" | jq '.data[].job.title'
```

Track them in the UI at http://127.0.0.1:3010/applications?email=ada@example.com.

## Theming

The web app ships light and dark themes. Tokens live in `app/globals.css` as RGB
triplets (`--brand`, `--ink`, …) re-declared under `.dark`; Tailwind maps them in
`tailwind.config.js`. An inline script in `app/layout.tsx` resolves the theme before
first paint, so there is no flash. Add a colour by declaring it in **both** blocks.

## Social login (optional)

Copy `web/nextjs/.env.example` to `web/nextjs/.env.local`, set `NEXTAUTH_SECRET` and
provider credentials, then restart `make web`. Sign-in UI lives at `/login`.

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
