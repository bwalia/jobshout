# Meta Lead Analyst

Builtin agent that pulls **Instagram / Facebook Lead Ads Instant Form**
submissions via the Meta Graph API, runs an LLM brief per lead plus a run
rollup, syncs contacts into **OpsAPI CRM**, and can ingest new leads in
realtime through Meta’s `leadgen` webhook.

Organic DMs and comment forms are out of scope.

## What you need

| Piece | Where |
| --- | --- |
| Meta app (App ID / App Secret) | [Meta for Developers](https://developers.facebook.com/) |
| Long-lived **Page** access token | Meta Business Suite → Page → settings / Graph API Explorer |
| Token scopes | `leads_retrieval`, plus Page ads/engagement scopes Meta documents for Lead Ads |
| Ring secrets | WSL Vault `jobshout/<ring>/meta` → K8s Secret `jobshout-meta` |
| Named connection | Agent tab → Connection (name + Page ID + token) |
| OpsAPI CRM | Same workspace as Marketing Manager (`marketing_api_key` with `crm` scope, or ring `OPSAPI_MARKETING_*`) |

Never put a Page token in the launch schema. Launch only takes the connection
**name** (and optional form / since / limit / focus / website / push_to_crm).

## Enable on int (ops)

1. Seed Vault path `jobshout/int/meta` with:
   - `META_APP_ID`, `META_APP_SECRET`
   - `META_WEBHOOK_VERIFY_TOKEN` (any strong shared string)
   - `META_TOKEN_KEY` (passphrase or 64-char hex — same style as `LINKEDIN_TOKEN_KEY`)
2. Uncomment `jobshout-meta` under `extraSecretRefs` and `vaultSecrets` in
   `deploy/helm/jobshout/values-int.yaml` (and prod when ready).
3. Deploy / restart `jobshout-api` after ESO syncs the Secret.
4. Confirm the agent tab Status shows **Seal tokens: yes**. Without
   `META_TOKEN_KEY`, connections cannot be stored (analyse/CRM code still
   degrades cleanly; Launch refuses with a clear error).

Webhook verify also needs `META_APP_SECRET` + `META_WEBHOOK_VERIFY_TOKEN`.

## Vault keys (`jobshout/<ring>/meta`)

| Env | Purpose |
| --- | --- |
| `META_APP_ID` | Meta app id (optional for Graph reads with a Page token; useful for ops) |
| `META_APP_SECRET` | Required to verify `X-Hub-Signature-256` on webhooks |
| `META_WEBHOOK_VERIFY_TOKEN` | Shared string for the GET hub challenge |
| `META_TOKEN_KEY` | AES key material for sealing Page tokens at rest |
| `META_GRAPH_VERSION` | Optional; default `v21.0` (also chart `meta.graphVersion`) |

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

Webhook path also pushes the analysed lead into OpsAPI CRM (skips high
`spam_risk` and leads with no email/phone).

## Connect a Page (manual token)

1. Mint a long-lived Page access token in Meta Business Suite / Graph Explorer.
2. Open the Meta Lead Analyst Workspace tab → **Connection**.
3. Save: connection **name** (e.g. `workstation-ig`), Page ID, optional Page
   name, and the token. The API stores only the sealed ciphertext.

## Launch / chat (via JobShout agent)

Schema fields (New task / Run task / `agent_execute`):

| Field | Required | Notes |
| --- | --- | --- |
| `connection` | yes | Named Page connection |
| `form` | no | Form ID or name; empty = forms on the Page (capped) |
| `since` | no | `7d` or RFC3339 |
| `limit` | no | Max leads (hard cap from `META_LEAD_MAX_LEADS`, max 200) |
| `focus` | no | Qualification angle for the LLM |
| `push_to_crm` | no | Default `yes` — create/update OpsAPI CRM contacts |
| `website` | no | OpsAPI website name / namespace for CRM; empty = org default |

Chat: call `agent_execute` on **Meta Lead Analyst** with `connection` (+
optional form / focus / website). Example values:
`connection=workstation-ig`, `since=7d`, `push_to_crm=yes`.

## CRM push (OpsAPI)

After analysis, each lead with an email or phone is upserted via
`POST /api/v2/crm/contacts` on the org’s Marketing Manager workspace
(dedupe by email). High `spam_risk` is skipped unless you force a re-push.

Re-push from the agent tab or `POST /api/v1/meta-leads/leads/{id}/push-crm`
with optional `{"website":"…","force":true}`.

Synced leads store `crm_contact_id` / `crm_website` / `crm_synced_at` (or
`crm_error` on failure).

## LLM output

Per lead: `intent`, `fit_score` (0–100), `next_action`, `spam_risk`, `flags[]`,
`summary`. Rollup: `themes[]`, `quality_mix`, `recommended_followups[]`,
`lead_count`. Decode is tolerant of odd JSON shapes.

Without an LLM provider the agent still stores leads and writes a heuristic
brief so ingest + CRM are not blocked.

## Out of scope

- Meta OAuth UI for connecting Pages (manual token + named connection first)
- Instagram Messaging / organic comment forms
- Auto email send from analyses (use Marketing Manager for campaigns)
