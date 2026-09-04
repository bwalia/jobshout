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

## Social login (optional)

Copy `web/nextjs/.env.example` to `web/nextjs/.env.local`, set `NEXTAUTH_SECRET` and
provider credentials, then restart `make web`. Sign-in UI lives at `/login`.

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
