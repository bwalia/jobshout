# Meta Lead Analyst

Builtin agent that pulls **Instagram / Facebook Lead Ads Instant Form**
submissions via the Meta Graph API, runs an LLM brief per lead plus a run
rollup, and can ingest new leads in realtime through Meta’s `leadgen` webhook.

Organic DMs and comment forms are out of scope.

## What you need

| Piece | Where |
| --- | --- |
| Meta app (App ID / App Secret) | [Meta for Developers](https://developers.facebook.com/) |
| Long-lived **Page** access token | Meta Business Suite → Page → settings / Graph API Explorer |
| Token scopes | `leads_retrieval`, plus Page ads/engagement scopes Meta documents for Lead Ads |
| Ring secrets | WSL Vault `jobshout/<ring>/meta` → K8s Secret `jobshout-meta` |
| Named connection | Agent tab → Connection (name + Page ID + token) |

Never put a Page token in the launch schema. Launch only takes the connection
**name** (and optional form / since / limit / focus).

## Vault keys (`jobshout/<ring>/meta`)

| Env | Purpose |
| --- | --- |
| `META_APP_ID` | Meta app id (optional for Graph reads with a Page token; useful for ops) |
| `META_APP_SECRET` | Required to verify `X-Hub-Signature-256` on webhooks |
| `META_WEBHOOK_VERIFY_TOKEN` | Shared string for the GET hub challenge |
| `META_TOKEN_KEY` | AES key material for sealing Page tokens at rest (same style as `GMAIL_TOKEN_KEY` / `LINKEDIN_TOKEN_KEY`) |
| `META_GRAPH_VERSION` | Optional; default `v21.0` |

Wire the Secret with `extraSecretRefs` + `vaultSecrets` (see
[wsl-vault-secrets.md](wsl-vault-secrets.md)). Restart `jobshout-api` after the
first sync.

## Webhook

Public routes (same pattern as Jira/GitHub):

- `GET  https://<host>/webhooks/meta` — Meta subscription verify
- `POST https://<host>/webhooks/meta` — `leadgen` notifications

In the Meta app:

1. Add the webhook callback URL above.
2. Use `META_WEBHOOK_VERIFY_TOKEN` as the verify token.
3. Subscribe the **Page** to the `leadgen` field.
4. Ensure a named JobShout connection exists for that Page ID so the handler
   can decrypt the Page token and fetch `GET /{leadgen_id}`.

## Connect a Page (first version: manual)

1. Mint a long-lived Page access token in Meta Business Suite / Graph Explorer.
2. Open the Meta Lead Analyst Workspace tab → **Connection**.
3. Save: connection **name** (e.g. `workstation-ig`), Page ID, optional Page
   name, and the token. The API stores only the sealed ciphertext.

## Launch / chat

Schema fields (also on New task / Run task / `agent_execute`):

| Field | Required | Notes |
| --- | --- | --- |
| `connection` | yes | Named Page connection |
| `form` | no | Form ID or name; empty = forms on the Page (capped) |
| `since` | no | `7d` or RFC3339 |
| `limit` | no | Max leads (hard cap from `META_LEAD_MAX_LEADS`, max 200) |
| `focus` | no | Qualification angle for the LLM |

Chat hint: call `agent_execute` on Meta Lead Analyst with `connection` (+
optional form / focus). Webhooks also analyse new leads for connected Pages.

## LLM output

Per lead: `intent`, `fit_score` (0–100), `next_action`, `spam_risk`, `flags[]`,
`summary`. Rollup: `themes[]`, `quality_mix`, `recommended_followups[]`,
`lead_count`. Decode is tolerant of odd JSON shapes.

Without an LLM provider the agent still stores leads and writes a heuristic
brief so ingest is not blocked.

## Out of scope (this version)

- Meta OAuth UI for connecting Pages (manual token + named connection first)
- Auto CRM push / outbound email from analyses
- Instagram Messaging / organic comment forms
