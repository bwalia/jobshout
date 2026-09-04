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
