# Mail Agent

JobShout’s Mail Agent watches **one shared organisation Gmail**, drafts
replies, and commissions the Research Agent when a reply needs current facts.
**Nothing is sent until a human approves the draft.**

This is not auto-reply, not per-user Gmail, and not a replacement for the SMTP
notification adapter.

## What it does

```
Inbox (poll / Sync now — Gmail `users.messages.list`, last 7 days)
  → Mail Agent classifies
      → optional: Research Agent (typed research.Request)
      → draft stored in JobShout (not sent)
  → Human edits, Approve or Reject
  → Approve calls Gmail send; Reject does not
```

The agent is seeded like Research / Security Tester (`builtin: mail`). New orgs
get it from registration; existing orgs from migration `000033_mail_agent`.

Sync talks to the Gmail REST API (`users.messages.list` / `get` / `send`) with
an OAuth refresh token. **Sync now** lists the last 7 days of INBOX mail
immediately and returns how many conversations Gmail matched. Watch senders
that contain spaces are quoted (`from:"Balinder Walia"`) so Gmail does not
parse them as two terms. Already-ingested threads are skipped; classify and
draft still run in the background reconciler.

## Google Cloud (ops prerequisite)

1. Google Cloud Console → APIs & Services → enable **Gmail API**.
2. Credentials → OAuth 2.0 Client ID, type **Web application**.
3. Authorised redirect URI must be exactly:

   `https://<host>/api/v1/mail/connection/oauth/callback`

   Local docker: `http://localhost:8190/api/v1/mail/connection/oauth/callback`

4. Put the client id, client secret, and a token-encryption key in the API
   process environment (cluster: an `extraSecretRefs` secret, never git).

## Environment

| Variable | Meaning |
|---|---|
| `GMAIL_CLIENT_ID` | OAuth client id. Empty disables Connect. |
| `GMAIL_CLIENT_SECRET` | OAuth client secret. Never logged. |
| `GMAIL_TOKEN_KEY` | Encrypts the refresh token at rest (AES-256-GCM). 64 hex chars or any passphrase. Generate with `openssl rand -hex 32`. |
| `GMAIL_OAUTH_REDIRECT_URL` | Must match the Google console URI. Helm sets `https://<host>/api/v1/mail/connection/oauth/callback`. |
| `FRONTEND_BASE_URL` | Where the browser is sent after OAuth (`/panel/task-manager?agent=mail`). |
| `MAIL_POLL_INTERVAL` | Time between automatic inbox polls (default `5m`). |
| `MAIL_RECONCILE_INTERVAL` | Reconciler tick (default `15s`) so “Sync now” is picked up quickly. |

Refresh tokens are stored as `bytea` ciphertext. They are never written to logs;
error strings that look like token query params are redacted.

## Scopes requested

| Scope | Why |
|---|---|
| `gmail.readonly` | Read inbox threads. |
| `gmail.send` | Send a reply **only after** Approve. |
| `userinfo.email` | Show the connected account. |

Drafts live in JobShout, so `gmail.compose` is not requested. Labelling /
archiving the mailbox is off unless `allow_mailbox_mutations` is turned on
(v1 still does not auto-label).

## Tables

- `mail_connections` — one row per org; encrypted refresh token
- `mail_oauth_states` — short-lived CSRF state for the OAuth redirect
- `mail_threads` — watched conversations + classification + research snapshot
- `mail_drafts` — JobShout-side draft; `draft` → `approved` → `sent`, or `rejected`

## Cluster secret

```bash
kubectl -n <ring> create secret generic jobshout-mail \
  --from-literal=GMAIL_CLIENT_ID=<id> \
  --from-literal=GMAIL_CLIENT_SECRET=<secret> \
  --from-literal=GMAIL_TOKEN_KEY=$(openssl rand -hex 32) \
  --dry-run=client -o yaml | kubectl apply -f -
```

Name that secret in `extraSecretRefs` for the ring. Rotating `GMAIL_TOKEN_KEY`
makes existing ciphertext unreadable; reconnect Gmail after a key change.

## Safety

- Send is `POST /api/v1/mail/drafts/{id}/approve` only. Any other status returns 403.
- Reject never calls Gmail send.
- Chat can sync the inbox and list pending drafts; it cannot send.
