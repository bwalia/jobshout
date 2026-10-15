# Runbook: JobShout secrets in WSLVault

On k3s1, Ring Promoter deploys with `externalSecret.enabled=false`, so
Helm-owned `jobshout-secrets` is placeholders only and is reset every deploy.
Real credentials live in **WSLVault** (KV v2 mount `kv`) and are synced by
ESO (`ClusterSecretStore` `wslvault-backend`) into per-purpose Secrets listed
in `extraSecretRefs`. Chart values commit **paths only**, never values.

This is **WSLVault**, not HashiCorp Vault. Use the WSLVault UI or the
`wslvault` CLI — do not use the HashiCorp `vault` binary against this host.

| | |
| --- | --- |
| UI login | https://vault-ui.workstation.co.uk/login |
| API | https://vault.workstation.co.uk |
| Docs | https://www.wslvault.org/ |
| KV mount | `kv` (versioned / Vault-compatible KV v2 API) |

Scoped API keys / tokens are created in the WSLVault UI. WSLVault does not
implement HashiCorp `auth/token/create` or `sys/policies`.

## Path layout

| K8s Secret | WSLVault path (under mount `kv`) | Expected env keys |
| --- | --- | --- |
| `jobshout-core` | `jobshout/<ring>/core` | `DATABASE_PASSWORD`, `JWT_SECRET`, `PYTHON_SIDECAR_SECRET` |
| `jobshout-cms` | `jobshout/<ring>/cms` | `OPSAPI_API_KEY` (CMS-only — no gateway/Gemini/MinIO keys) |
| `jobshout-research` | `jobshout/<ring>/research` | `BRAVE_SEARCH_API_KEY`, optional research `GITHUB_TOKEN` |
| `jobshout-review` | `jobshout/<ring>/review` | `GITHUB_TOKEN`, `REVIEW_BOT_TOKEN` |
| `jobshout-mail` | `jobshout/<ring>/mail` | `GMAIL_CLIENT_ID`, `GMAIL_CLIENT_SECRET`, `GMAIL_TOKEN_KEY` |
| `jobshout-google` | `jobshout/<ring>/google` | `GOOGLE_OAUTH_CLIENT_ID`, `GOOGLE_OAUTH_CLIENT_SECRET` |
| `jobshout-linkedin` | `jobshout/<ring>/linkedin` | `LINKEDIN_CLIENT_ID`, `LINKEDIN_CLIENT_SECRET`, `LINKEDIN_TOKEN_KEY` |
| `jobshout-wslproxy` | `jobshout/<ring>/wslproxy` | `WSLPROXY_API_TOKEN` and/or `WSLPROXY_USERNAME`+`WSLPROXY_PASSWORD`; optional `WSLPROXY_MCP_API_KEY` |
| `jobshout-secretsrot` | `jobshout/<ring>/secretsrot` | `SECRETS_VAULT_ADDR`, `SECRETS_VAULT_TOKEN`, `RP_API_TOKEN`; optional `SECRETS_ROT_GITHUB_TOKEN` |
| `jobshout-gemini` | `jobshout/<ring>/gemini` | `GEMINI_API_KEY` |
| `jobshout-gateways` | `jobshout/<ring>/gateways` | `OLLAMA_JWT_SECRET`, `IMAGE_JWT_SECRET` |
| `jobshout-strix` | `jobshout/<ring>/strix` | `STRIX_JWT_SECRET` (separate from gateways on purpose) |

`<ring>` is `int`, `test`, `acc`, or `prod`. Keys must match env names exactly
(`GEMINI_API_KEY`, not `geminiApiKey`).

`jobshout-secretsrot-kube` (kubeconfig file mount) stays out-of-band for now.

## Populate a path (before first ESO sync)

Seed from the live k8s Secret when migrating. Keep values in your terminal or
the UI only — never paste them into git, tickets, chat, or PR bodies.

### UI

1. Sign in at https://vault-ui.workstation.co.uk/login.
2. Open the `kv` engine and create (or edit) the path, e.g. `jobshout/int/gemini`.
3. Add each env key with its value (exact names from the table above).
4. For `jobshout/int/cms`, write **`OPSAPI_API_KEY` only** — drop any
   gateway/Gemini/MinIO keys that drifted into the old hand-applied Secret.

Read a value locally to paste into the UI:

```bash
kubectl -n int get secret jobshout-gemini -o jsonpath='{.data.GEMINI_API_KEY}' | base64 -d
# paste into WSLVault UI; do not echo into shared logs
```

### `wslvault` CLI

Install (macOS / Linux, needs Rust):

```bash
cargo install --git https://github.com/bwalia/wslvault --locked wslvault-cli
# binary: ~/.cargo/bin/wslvault
```

Configure once (token/API key from the WSLVault UI — not HashiCorp `vault login`):

```bash
wslvault init
# endpoint: https://vault.workstation.co.uk
# writes ~/.wslvault/config.toml
#
# or non-interactive:
#   export WSLVAULT_ADDR=https://vault.workstation.co.uk
#   export WSLVAULT_TOKEN=…   # from vault-ui
#   export WSLVAULT_TENANT_ID=…
```

Seed a path (real subcommand is `secret`, not HashiCorp-style `kv`):

```bash
GEMINI_API_KEY=$(kubectl -n int get secret jobshout-gemini -o jsonpath='{.data.GEMINI_API_KEY}' | base64 -d)
wslvault secret put jobshout/int/gemini --data GEMINI_API_KEY="$GEMINI_API_KEY"
unset GEMINI_API_KEY

# confirm (avoid sharing full output)
wslvault secret get jobshout/int/gemini --format json | jq 'keys'
```

Optional / missing secrets (`research`, `linkedin`) can wait; refs are
`optional: true`. Populate **gemini / gateways / strix / cms / core** before
merge so ExternalSecrets can sync on deploy.

## After chart deploy: sync and restart

1. Wait for ESO (or force-sync):
   ```bash
   kubectl -n <ring> annotate externalsecret <name> force-sync="$(date +%s)" --overwrite
   kubectl -n <ring> get externalsecret <name>
   # STATUS=SecretSynced, READY=True
   ```
2. Confirm ESO owns the Secret (`ownerReferences` → ExternalSecret). Only then
   remove stale `kubectl.kubernetes.io/last-applied-configuration` annotations
   from hand-applied secrets. Chart `deletionPolicy: Retain` — removing a
   `vaultSecrets` entry does not delete the Secret.
3. Restart the API so it reloads env (prefer Ring Promoter with an explicit
   deployments list — never restart by `app.kubernetes.io/instance=jobshout`
   alone):
   ```bash
   # POST /api/apps/jobshout/rings/<ring>/restart
   # body: {"deployments":["jobshout-api"]}
   ```
4. Verify key presence only, not values:
   ```bash
   kubectl -n <ring> exec deploy/jobshout-api -- sh -c \
     'for k in GEMINI_API_KEY OPSAPI_API_KEY IMAGE_JWT_SECRET; do
        eval v=\$$k; echo "$k=$([ -n \"$v\" ] && echo SET || echo unset)"; done'
   ```

## Boundaries

- Keep `externalSecret.enabled=false` for Helm `jobshout-secrets` on k3s1.
- Do not put credentials in agent launch-form schema fields.
- `jobshout-strix` stays separate from `jobshout-gateways`.
- `jobshout-cms` holds CMS keys only so overlays do not surprise other rings.
