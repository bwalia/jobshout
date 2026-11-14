# Deploy identity (footer / sidebar)

Operators see a one-line stamp on every JobShout web surface that has a footer:

```text
int · v1.0.201 · c82fc12 · Build #43474661245 · 13 Nov 2026, 17:38 GMT
```

Missing fields are omitted — nothing is invented locally.

## Fields and sources

| Field | Example | Source |
| --- | --- | --- |
| Ring / env | `int` | Helm `APP_ENV` (`values.env`) |
| Version / tag | `v1.0.201` | Helm `APP_VERSION` (`image.tag` / GitHub release tag) |
| Short SHA | `c82fc12` | CI `GITHUB_SHA` → `APP_GIT_SHA` / binary ldflags |
| Build # | `43474661245` | CI `GITHUB_RUN_ID` → `APP_BUILD_ID` / binary ldflags |
| Deployed at | `13 Nov 2026, 17:38 GMT` | Helm `APP_DEPLOYED_AT` (chart render time, RFC3339) |

## Platform (`web/nextjs`)

- UI reads `GET /health` (`version`, `env`, `git_sha`, `build_id`, `deployed_at`).
- API resolves:
  - `APP_VERSION` (Helm) over binary `main.version`
  - `APP_ENV`, `APP_DEPLOYED_AT` from Helm configmap
  - `APP_GIT_SHA` / `APP_BUILD_ID` env if set, else ldflags from image build
- CI (`deploy-k3s.yml`) passes `--build-arg VERSION`, `GIT_SHA=$GITHUB_SHA`, `BUILD_ID=$GITHUB_RUN_ID` into `server/Dockerfile`.

## JobShout.com (`jobshout-com/web/nextjs`)

- `SiteFooter` reads process env at SSR (`lib/deploy-identity.ts`).
- Image bake (`Dockerfile.web`): `APP_GIT_SHA`, `APP_BUILD_ID`, fallback `APP_VERSION`.
- Helm `web.yaml` sets per ring: `APP_VERSION`, `APP_ENV`, `APP_DEPLOYED_AT`.
- CI (`deploy-jobshout-com.yml`) passes the build-args when building the web image.

## Local / compose

Optional in `docker-compose.yml` for the API: `APP_VERSION`, `APP_ENV`, `APP_GIT_SHA`, `APP_BUILD_ID`, `APP_DEPLOYED_AT`. Leave unset for a quiet stamp.
