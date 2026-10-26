# Marketing Manager

The Marketing Manager (`builtin: marketing_manager`) runs an org's **OpsAPI
Marketing Cloud** from JobShout chat: email templates, CRM contacts, lists and
segments, campaigns, test sends, sending and scheduling, and results.
**Nothing reaches a person without the user's confirmation, and a ring that
does not allow live sending can only send from a workspace in test mode.**

Package: `server/internal/marketingmanager` (chat tools in `…/chattools`).
It is registered like every specialist (`.claude/rules/agent-modules.md`):
one `agentmodules.Register` entry, its chat tools attached with
`agentmodule.SetToolInstaller`. No platform switch names it.

## How it fits

```
chat ("Prepare the launch campaign for opted-in customers")
  → marketing_read    status, templates, lists, audience preview, analytics…
  → marketing_write   create/edit templates, contacts, lists, draft campaigns, validate
  → marketing_send    test send, schedule/send, resume, cancel, import, consent
        ↑ the chat guard holds every call for the user's approval, showing
          workspace, sender, subject, content, audience list and eligible
          count, schedule, and TEST MODE or LIVE
  → OpsAPI /api/v2/opsapi-marketing/* and /api/v2/crm/contacts
```

- **One tool per kind.** The service refuses an action called through the
  wrong tool, so a send can't ride in on `marketing_write` and skip approval.
- **A fixed operation table** (`ops.go`). The model picks an operation by
  name and supplies ids/query/body. It never supplies a URL or method. Ids are
  validated before any request, and requests can't leave the configured host
  or follow redirects.
- **Run it as a task** (New task / Run task / `agent_execute`) for a written
  **review** of recent campaigns (delivery, bounce, open (estimated), click,
  click-to-open, skipped, A/B winner, plus suggestions) or a **set-up check**.
- **Email Writer → Manager.** The Marketing Email Writer saves its mail-merge
  template to the same Marketing Cloud (`opsapi/marketing.go`). The Manager
  finds it with `list_templates` and builds a campaign from it. The two use the
  same OpsAPI endpoints. The Email Writer's adapter covers three template
  calls; the Manager needs about 60 routes, idempotency keys and retries, so it
  has its own table-driven client rather than a second copy of each call.

## Safety

| Rule | Where |
| --- | --- |
| Sends, scheduling, resume, cancel, CSV import, opt-in/opt-out, unblocking an address need the user's confirmation every time | `marketing_send` is `Destructive`; chat guard |
| The confirmation shows workspace, sender, subject, content source, audience list, eligible/excluded counts, schedule and test/live mode | `gate.go` `Describe` |
| A delivering call goes ahead only if the workspace is in **test mode** (OpsAPI `dry_run`), or the server sets `MARKETING_LIVE_SEND=true` **and** the call carries `live_send_approved` (which the user saw and approved). Test mode that can't be read counts as live | `gate.go` `gate` |
| Test sends go only to confirmed test recipients (OpsAPI's own test-send accepts any address) | `precheck.go` |
| Consent, unsubscribes, suppression, quiet hours, frequency caps, merge-field and compliance validation are enforced by OpsAPI on every send. The agent can't override them | OpsAPI `can_send` |
| The agent can't approve campaigns (four-eyes), change test mode or the sender, or delete campaigns, lists, templates or contacts: those operations aren't in the table **and** the key lacks the scopes | `ops.go`, key profile |
| Creates send an `Idempotency-Key`; a contact whose email exists is returned, not duplicated; only GETs and idempotent creates are retried, never a send | `service.go`, `client.go` |
| Every write and send, refused ones included, goes to `audit_logs` (`marketing.<action>`, field names only, never bodies or personal data) | `chattools/tools.go` |
| The API key never appears in results, errors or logs | `client.go` `scrub` |
| Tenant isolation: each call resolves the org's own workspace afresh. A named website the org doesn't have is an error. OpsAPI binds a key to one namespace | `resolve.go` |

## Configuration

A workspace comes from (first match):

1. `website` named in the call: the org's OpsAPI integration with that name
   (or namespace slug);
2. the org's only OpsAPI integration with `credentials.marketing_api_key`;
3. the ring's own workspace: `OPSAPI_MARKETING_API_KEY` +
   `OPSAPI_MARKETING_NAMESPACE` (default `OPSAPI_NAMESPACE`) on
   `OPSAPI_BASE_URL`.

| Setting | Meaning |
| --- | --- |
| `OPSAPI_MARKETING_API_KEY` | Ring workspace key, marketing profile. **Never the CMS key** |
| `OPSAPI_MARKETING_NAMESPACE` | Its namespace |
| `MARKETING_TEST_RECIPIENTS` | Comma list of confirmed test addresses (added to each website's own `config.marketing_test_recipients`) |
| `MARKETING_LIVE_SEND` | `true` lets sends leave a workspace that is not in test mode. Leave unset on int/test/acc |
| integration `credentials.marketing_api_key` | Per-org key (falls back to `api_key`). Needs `SECRETS_ENCRYPTION_KEY` on the ring, or the server refuses to store it |
| integration `config.marketing_brand` | Optional colours/logo/font/tone, returned by `status` for designing emails |

Mint a key (a namespace admin runs this; the key is shown once):

```bash
OPSAPI_KEY_PROFILE=marketing OPSAPI_BASE_URL=https://int-opsapi.workstation.co.uk \
OPSAPI_NAMESPACE=workstation ./scripts/opsapi-api-key.sh
```

Scopes: `crm` read/create/update; `opsapi_marketing_templates|lists|campaigns`
read/create/update; `opsapi_marketing_analytics|settings|conversations` read;
`opsapi_marketing_consent` read/create/delete. Deliberately **not**
`opsapi_marketing_campaigns.manage` (approve) or
`opsapi_marketing_settings.update` (test mode, sender).

On k3s1, int reads the ring settings from WSL Vault `jobshout/int/marketing`
(`jobshout-marketing` in `values-int.yaml`, see `docs/wsl-vault-secrets.md`).
Other rings don't have it yet.

## Testing on INT (test mode only)

1. In int OpsAPI, namespace `workstation` (or a scratch namespace): Workspace →
   Plugins → Marketing Cloud → turn on **Test mode: don't really send**. Keep
   it on for the whole test.
2. Mint a marketing-profile key (above).
3. In WSL Vault, `kv` → `jobshout/int/marketing`: `OPSAPI_MARKETING_API_KEY`,
   `OPSAPI_MARKETING_NAMESPACE=workstation`,
   `MARKETING_TEST_RECIPIENTS=<your address>`. Force-sync and restart the API
   with an explicit deployments list:
   ```bash
   kubectl -n int annotate externalsecret jobshout-marketing force-sync="$(date +%s)" --overwrite
   # Ring Promoter: POST /api/apps/jobshout/rings/int/restart {"deployments":["jobshout-api"]}
   ```
   The boot log line `marketing manager initialised` should show
   `ring_workspace=true live_send_allowed=false`.
4. In int JobShout chat:
   - "Check the marketing set-up". Expect *test mode (dry run)*.
   - "Create a Workstation Shop launch email template" → a draft template.
   - "Add jane@example.com, Jane Doe, to a list called Launch test". Repeat
     it: the contact is reused, not duplicated.
   - "Create a campaign for Launch test using that template, and validate it."
   - "Show me a preview and send a test email" → approval card → test send to
     your confirmed address only. Try another address: refused.
   - "Send it now" → the approval card shows workspace, sender, subject,
     content, list and eligible count, schedule, **TEST MODE**. Approve: the
     result says `send_mode: dry_run`, and OpsAPI records deliveries but sends
     nothing.
   - "Show last month's campaign performance".
5. Audit trail: `SELECT action, metadata FROM audit_logs WHERE action LIKE 'marketing.%' ORDER BY created_at DESC`.

## Not supported (OpsAPI has no API for it)

- **Triggered automations / drip sequences** (for example, "3 days after
  signup"). The Marketing Cloud has scheduled one-off campaigns, batching and
  A/B tests, but no workflow engine. Follow-ups are new campaigns (duplicate +
  segment on engagement) and must be scheduled each time.
- **Tags.** Segments filter on CRM and engagement fields (`segment_fields`);
  there's no tag model.
- **Server-side contact dedupe.** OpsAPI CRM create doesn't dedupe; the agent
  checks by exact email first, which leaves a small window for concurrent
  creates.
- **Approving campaigns, changing test mode/sender/limits, deleting records.**
  These stay with people in OpsAPI by design.
- **Imports** are capped at 2000 rows per call (OpsAPI's limit) and need the
  `email` column.
- Opens are pixel-based estimates; treat open rates as a prompt, not a verdict.
