# JobShout

- **New specialists** must register, not special-case the platform. Rule: `.claude/rules/agent-modules.md` (Cursor: `.cursor/rules/agent-modules.mdc`).

## CI and merging

- `master` is protected: "Go build, vet & test", "Web typecheck & lint" and "PR image check" must pass, admins included. Never merge a red PR — #187 did, and broke every deploy until #189.
- Before pushing web changes, run `cd web/nextjs && npm run typecheck && npm run lint`. A missing import otherwise only surfaces an hour into the image cross-build.
- GitGuardian scans every commit in a PR, not just the head. Build credential-looking test fixtures at runtime (`strings.Join`) instead of literals.
- Migrations replay on every boot: `IF NOT EXISTS` / `WHERE NOT EXISTS` only. Take the next free number from `origin/master`, not your branch.

## Rings and deploys (Ring Promoter, k3s1)

- A merge to master deploys int. Ring Promoter auto-promotes int → test → acc when healthy; prod is manual and needs the production password.
- `jobshout` uses RP's **k8sjob** deployer (a Job in `ring-exec` runs `helm upgrade`). Re-seeding the running version is a no-op; to pick up changed secrets use `POST /api/apps/jobshout/rings/{ring}/restart` with an explicit `deployments` list. `app.kubernetes.io/instance=jobshout` also matches redis/langfuse/minio — never restart by that label.

## Secrets on k3s1

- ESO is installed (serves `external-secrets.io/v1` only) with ClusterSecretStore `wslvault-backend` → WSLVault `kv` mount. But the chart is installed with `externalSecret.enabled=false`, so `jobshout-secrets` is **Helm-owned** — patching it is undone by the next deploy. `extraSecretRefs` Secrets are not Helm-managed and are safe to patch.
- WSLVault speaks the KV v2 API but not `auth/token/create` or `sys/policies`; scoped tokens must be made in its UI.
- Launch-form values are persisted to the task (`TaskMetaLaunchValues`). Never make a credential a schema field — reference server-side creds by name.

## Article Writer

- `BLOG_MAX_RUNTIME` is a per-article budget (one article ≈ 25–30 min on int). A flat per-run cap kills finished runs.
- Keep discovery's seed URLs and focus areas on the brief through research and planning; research that starts from the topic wording alone drifts off-subject.
- Model JSON is loosely shaped: decode optional/hint fields tolerantly (see `flexibleString`) so an odd shape can't fail a whole run.
