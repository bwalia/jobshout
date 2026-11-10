# SEO Analyst

Keyword-driven SEO specialist: audit public URLs one-by-one, or analyse HTML/Markdown
files in a GitHub repo via a **named connection**. Tokens are sealed at rest and never
appear on the launch schema.

## Modes

| Source | What it does |
| --- | --- |
| `urls` | Fetch each URL; score title/meta/H1/OG/robots/sitemap vs the keyword set; rollup report |
| `github` | List repo files matching `path_glob`, read via GitHub API, score each page; propose frontmatter/meta patches |

Actions (`mode`): `analyze` (report), `improve` (propose patches), `publish` (CMS API or open a PR).

## Named GitHub connection

Save credentials in the agent Workspace → **Connection** tab (or `PUT /api/v1/seo/connections`):

- `name` — referenced as `github_connection` on launch
- `repo` — `owner/repo`
- `default_branch` — usually `main`
- `access_token` — PAT with `contents:read` (and `pull_requests:write` to open PRs)

Sealed with `SEO_TOKEN_KEY` (AES-GCM via the same helper as Mail / Meta Lead). Falls back to
`META_TOKEN_KEY` or `GMAIL_TOKEN_KEY` if unset.

### Vault / ring

Store on WSL Vault under `jobshout/<ring>/seo` (or reuse an existing seal key secret):

| Key | Purpose |
| --- | --- |
| `SEO_TOKEN_KEY` | Passphrase or 64-char hex for sealing PATs |

Wire via `vaultSecrets` + `extraSecretRefs` like other agents (`docs/wsl-vault-secrets.md`).
After sync, restart `jobshout-api` with an explicit deployments list.

## Launch schema (no secrets)

- `source` — `urls` \| `github`
- `keywords` — required textarea
- `urls` — newline/comma list (URL mode)
- `github_connection` — connection name (GitHub mode)
- `path_glob`, `max_pages`, `git_repo`, `git_branch`, `mode`, `platform`, `sitemap_path`, `instruction`

## Report shape

Each run stores:

- `progress` — `{ total, completed, current, phase }`
- `pages[]` — per-page status, score, issues, `proposed_diff`
- `report` — rollup (first page detail + aggregated issues/keywords)
- `publish` — PR URL when publish succeeds, or `suggested_diff` channel on improve

GitHub publish writes `SEO_PATCH.md` plus `seo-patches/<path>.md` proposal files on a new
branch and opens a PR when the connection token has write scope.

## Try on int

1. Ensure `SEO_TOKEN_KEY` (or a fallback seal key) is present on the int API pod.
2. Open **Agents → SEO Analyst → Workspace**.
3. **Connection**: save `name=demo`, `repo=owner/repo`, PAT.
4. **Run**: source=URL audit, paste 1–2 URLs + keywords → Analyse.
5. Or source=GitHub, pick connection, glob `**/*.md`, max pages 5 → Analyse / Propose / Publish.

## API

- `GET /api/v1/seo/status`
- `GET|PUT /api/v1/seo/connections`, `DELETE /api/v1/seo/connections/{name}`
- `GET|POST /api/v1/seo/runs`, `GET /api/v1/seo/runs/{id}`, `POST .../cancel`
