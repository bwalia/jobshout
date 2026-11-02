# Meta Lead Analyst

Builtin agent that finds sales leads on a connected **Facebook Page** and its
linked **Instagram professional account**:

- **DMs, comments and @mentions** arrive by webhook and are qualified one by
  one (a DM thread is one lead per sender, re-qualified as messages arrive).
- **Lead Ads Instant Form** submissions are pulled on launch (backfill) or
  arrive through the `leadgen` webhook, with a run rollup.

Real leads with an email or phone sync into **OpsAPI CRM**.

## What you need

| Piece | Where |
| --- | --- |
| Meta app (App ID / App Secret) | [Meta for Developers](https://developers.facebook.com/) |
| Long-lived **Page** access token | Meta Business Suite → Page → settings / Graph API Explorer |
| Token scopes | Forms: `leads_retrieval`. DMs/comments/mentions: see [Permissions](#permissions) |
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
| `META_GRAPH_VERSION` | Optional; default `v26.0` (also chart `meta.graphVersion`) |

## Webhook

Public routes (same pattern as Jira/GitHub):

- `GET  https://<host>/webhooks/meta` — Meta subscription verify
- `POST https://<host>/webhooks/meta` — leadgen, DM, comment and mention notifications

The handler checks `X-Hub-Signature-256`, answers 200 at once and processes
the events in the background (Meta retries a slow answer, and each event can
be an LLM call). A retried delivery is recognised by message / comment id and
not analysed twice.

In the Meta app (App Dashboard → Webhooks), set the callback URL above and
`META_WEBHOOK_VERIFY_TOKEN`, then subscribe:

| Object | Fields | Becomes |
| --- | --- | --- |
| **Page** | `leadgen` | Lead Ads form lead |
| **Page** | `messages` | `facebook_dm` (one lead per sender) |
| **Page** | `feed` | `facebook_comment` (new comments and visitor posts) |
| **Page** | `mention` | `facebook_mention` (text read from Graph) |
| **Instagram** | `messages` | `instagram_dm` (one lead per sender) |
| **Instagram** | `comments` | `instagram_comment` |
| **Instagram** | `mentions` | `instagram_mention` (text read from Graph) |

Then install the app on the Page, which also turns on Instagram webhooks for
the linked account (run with the Page token, e.g. in Graph API Explorer):

```bash
curl -X POST "https://graph.facebook.com/v26.0/<PAGE_ID>/subscribed_apps" \
  -d "subscribed_fields=leadgen,messages,feed,mention" \
  -d "access_token=<PAGE_TOKEN>"
```

Every event is matched to a JobShout connection by the entry's id: the Page
id, or for Instagram the Page's linked account id, which is looked up when the
connection is saved (the Connection tab shows it). Events for an account with
no connection are ignored.

## DMs, comments and mentions

Each interaction gets one LLM brief with fixed definitions (the prompt is in
`server/internal/metalead/analyse.go`):

| Field | Meaning |
| --- | --- |
| `is_lead` | Wants to buy, order, book, get a price or quote, or propose paid business. Support, complaints, praise, friend tags, emoji-only and spam are not leads |
| `category` | `buying`, `pricing`, `booking`, `partnership`, `support`, `complaint`, `spam`, `other` |
| `fit_score` | 80+ clear intent with specifics · 50–79 interest, details missing · 20–49 vague (a bare "price?") · <20 not a lead |
| `evidence` | The message's own words that show the intent. Dropped (flag `evidence_not_in_message`) if the message does not contain it |
| `intent`, `next_action`, `summary`, `spam_risk`, `flags` | As for form leads |

Emails and phone numbers come from the message text by pattern, never from the
model. CRM push needs `is_lead` true, `spam_risk` not high, and an email or
phone in the thread; a DM thread that only gets an email in its third message
is pushed then. Without an LLM provider an interaction is stored with a
"Review manually" brief.

The model is `META_LEAD_MODEL` or the org's agent model: an env change, not a
code change. Before trusting a model, label a few hundred real DMs and
comments and compare candidates on precision, recall and run-to-run agreement.

## Permissions

The Page token needs, beyond `leads_retrieval` for forms:

| For | Permissions |
| --- | --- |
| Facebook DMs | `pages_messaging`, `pages_manage_metadata` |
| Facebook comments, mentions | `pages_read_engagement`, `pages_read_user_content`, `pages_manage_metadata` |
| Instagram DMs | `instagram_basic`, `instagram_manage_messages`, `pages_manage_metadata`, `pages_read_engagement`, `pages_show_list` |
| Instagram comments, mentions | `instagram_basic`, `instagram_manage_comments`, `pages_manage_metadata`, `pages_read_engagement`, `pages_show_list` |

With **Standard Access** (an app in Development mode) Meta only delivers
events from people with a role on the app — enough to test with your own
accounts. Real customers' DMs and comments need **Advanced Access** for each
permission, which means App Review (screencasts of the flow) and Business
Verification. Start those early; they take weeks.

Instagram DMs also need *Allow access to messages* switched on in the
Instagram app (Messages and story replies → Connected tools).

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
- Backfilling DMs, comments and photo tags from before the webhook was on
  (Graph `conversations`, comments and `/tags`): webhooks only see new events
- Replying from JobShout (Meta allows replies within 24 hours of the person's
  last message, and one private reply per comment)
- Auto email send from analyses (use Marketing Manager for campaigns)
