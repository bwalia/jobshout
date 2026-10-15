# Runbook: JobShout secrets in WSL Vault

On k3s1, Ring Promoter deploys with `externalSecret.enabled=false`, so
Helm-owned `jobshout-secrets` is placeholders only and is reset every deploy.
Real credentials live in **WSL Vault** (KV v2 mount `kv`) and are synced by
ESO (`ClusterSecretStore` `wslvault-backend`) into per-purpose Secrets listed
in `extraSecretRefs`. Chart values commit **paths only**, never values.

## Path layout

| K8s Secret | Vault path (KV v2 under `kv`) | Expected env keys |
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

## Add or rotate a secret

1. Write or update the KV v2 path in the WSL Vault UI (or CLI). Seed from the
   existing k8s Secret once if migrating; do not paste values into git, tickets,
   or PR bodies.
2. Ensure `values-<ring>.yaml` lists the name in both `extraSecretRefs` and
   `vaultSecrets` (`store: wslvault-backend`, `key: jobshout/<ring>/<name>`).
3. After deploy (or annotate to force-sync):
   ```bash
   kubectl -n <ring> annotate externalsecret <name> force-sync="$(date +%s)" --overwrite
   kubectl -n <ring> get externalsecret <name>
   # STATUS=SecretSynced, READY=True
   ```
4. Confirm ESO owns the Secret (`ownerReferences` → ExternalSecret). Only then
   remove stale `kubectl.kubernetes.io/last-applied-configuration` annotations
   from hand-applied secrets. Chart `deletionPolicy: Retain` — removing a
   `vaultSecrets` entry does not delete the Secret.
5. Restart the API so it reloads env (prefer Ring Promoter with an explicit
   deployments list — never restart by `app.kubernetes.io/instance=jobshout`
   alone):
   ```bash
   # example: POST /api/apps/jobshout/rings/<ring>/restart
   # body: {"deployments":["jobshout-api"]}
   ```
6. Verify key presence only, not values:
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
