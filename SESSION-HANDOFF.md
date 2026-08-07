# JobShout — Session Handoff (2026-07-25)

Saved work-state so this can be resumed. Covers the deployment saga, the
agent-platform build-out, the UI redesign, and the Ring Promoter deploy.

## TL;DR — current state
- **jobshout is deployed across the full DTAP pipeline** (int / test / acc / prod)
  on **k3s1**, all rings at image tag **`89f91ac`**, **healthy** in Ring Promoter.
- **Public:** `https://int.jobshout.co.uk` is live (login page + API through the
  nginx gateway).
- **Deploy mechanism:** `git push master → CI builds+pushes images → seeds Ring
  Promoter int → RP k8sjob deploys`. Merged and durable. Promotion
  `int→test→acc→prod` is a click in the RP UI (`rp.workstation.co.uk/?app=jobshout`).
- **4 open PRs** (agent-platform Phases 1–3 + the UI redesign) await review.

---

## 1. Deployment — how it works now (ALL MERGED)

**Flow:** push to `master` → `.github/workflows/deploy-k3s.yml`:
1. `build` job: cross-builds (buildx `--platform linux/amd64`, arm64 Mac runner)
   and pushes 3 images to `int-spectoncr.diytaxreturn.co.uk/jobshout/{server,web,python-sidecar}:<sha>`.
2. `deploy` job: `POST rp.workstation.co.uk/api/apps/jobshout/seed {ring:int, version:<sha>}`.
3. Ring Promoter runs a **k8sjob** in `ring-exec` → git-clones jobshout at the tag →
   `helm upgrade --install jobshout deploy/helm/jobshout -f values-<ring>.yaml
   --set image.tag=<sha> --set externalSecret.enabled=false --set-string
   secrets.registry{Username,Password}=<from ring-exec secret>`.
4. Promote `int→test→acc→prod` in the RP UI.

**Key facts / gotchas discovered (all fixed):**
- Runner is an **arm64 Mac**, k3s1 nodes are **amd64** → must cross-build (buildx).
- Go compiler **segfaults under QEMU** → server Dockerfile cross-compiles natively
  (`--platform=$BUILDPLATFORM` + `GOARCH=$TARGETARCH`).
- Cluster has **no External Secrets Operator** → deploy with
  `--set externalSecret.enabled=false` + plaintext registry/secret fallback.
- `SERVER_PORT` must be `":8080"` (leading colon) not `"8080"`.
- The **wslproxy edge routes the whole host to ONE backend**, so the chart uses an
  in-cluster **nginx gateway** (`jobshout-nginx`) that fans out `/api` + `/webhooks`
  + `/health` → api, everything else → web. Ingress is a single `/ → jobshout-nginx`.
- Registry `spectoncr.workstation.co.uk` is NOT reachable from the runner;
  `int-spectoncr.diytaxreturn.co.uk` (pop1) IS — same registry.
- Fresh-namespace api crash-loop (connects to Postgres before it's up) → **fixed by
  the `wait-for-postgres` initContainer** (PR #23).

**Merged deploy PRs (jobshout):** #6, #8–#17, #22, #23.
**Merged (ring-promoter):** #60/#61 (register jobshout k8sjob app), #62 (ESO-off +
registry creds in the k8sjob).

## 2. DNS / edge (int.jobshout.co.uk)
- Cloudflare **A record** `int.jobshout.co.uk → 187.124.112.155` (pop0), provisioned
  via the wslproxy admin API (`POST /api/dns/provision`, its own CF token).
- wslproxy **vhost** on pop0 for the host, routing **rule
  `8f161403-8592-1111-6294-9c57974505b0`** → the k3s1 Traefik backend (same rule
  rp.workstation.co.uk uses). Admin gateway: `https://prod-our-v1.wslproxy.com`,
  login `debian@wslproxy.org`.
- Edge specs committed at `deploy/edge/wslproxy-server-*.json`; the
  `register-edge-vhost.yml` workflow automates CNAME + vhost.
- test/acc/prod hosts (`test.jobshout.co.uk`, …) are NOT registered on the edge yet —
  only int has the public host. They ARE deployed in-cluster though.

## 3. Agent platform build-out (OPEN PRs — review needed)
Leena.ai-style "automation agents". Stack: #18 → #19 → #20 (merge in order).
- **#18 Phase 1 — tool ecosystem:** integrations (Jira/GitHub/Slack/Teams/Email)
  become agent-callable tools; **MCP client** (`mcp_list_tools`/`mcp_call`, table
  `mcp_servers`); **native LLM function-calling** with JSON-Schema (`SchemaProvider`).
- **#19 Phase 2 — RAG / Knowledge Studio:** `llm.Embedder` (OpenAI/Ollama),
  `000016_knowledge_vectors` (pgvector + `knowledge_chunks`), ingestion + chunking,
  `knowledge_search` tool + auto-injection into the executor, semantic memory
  (`000017_memory_embeddings`, ILIKE→cosine). **⚠ Requires `CREATE EXTENSION vector`
  on the DB.**
- **#20 Phase 3 — human-in-the-loop approvals:** `000018_approvals`, gated tool
  calls pause the executor + notify the agent's `manager_id`, resume via
  `POST /api/v1/approvals/{id}/decide`. Native-path pause is a follow-up.
- All build/vet/test green. Remaining plan: **Phase 4** (event triggers / always-on),
  **Phase 5** (evaluator-LLM guardrails). Prompt saved conceptually — see the
  gap-analysis in the session.

## 4. UI redesign (OPEN PR #21) — "Signal Room"
- Ownable mission-control identity: **warm-ink** surfaces + **Signal Amber** brand
  (green/red/blue = status only), **Space Grotesk / Inter / JetBrains Mono**
  (mono-for-telemetry), signature **`SignalDot`** live pulse.
- Done: tokens (reskins all 24 screens via CSS-var cascade), fonts, `SignalDot`,
  `AgentStatusBadge`, dashboard `MetricCard`, sidebar brand. `next build` = 24/24 ✓.
- Remaining: per-screen bespoke layout polish (dashboard grid, agent-board,
  workflows, org-builder).

## 5. Ops / access notes
- **kubeconfig:** `~/.kube/k3s1.yaml` (KUBECONFIG). Cluster is k3s1 (9 amd64 Debian nodes).
- **RP API token:** `kubectl -n workstation-ring-promoter get secret ring-promoter -o jsonpath='{.data.RP_API_TOKEN}' | base64 -d`
- **Seed a ring:** `curl -X POST rp.workstation.co.uk/api/apps/jobshout/seed?async=1 -H "Authorization: Bearer $TOK" -d '{"ring":"int","version":"<sha>"}'`
- **Promote:** `POST .../promote?async=1 {"from_ring":"int"}` (→ deploys next ring).
- **ring-exec/jobshout-registry-creds** secret (registry user/pass) was created
  manually; documented in ring-promoter `deploy/k8s/configmap.yaml`. If ring-exec is
  recreated, recreate it.
- jobshout GitHub secrets set: `REGISTRY_USERNAME`, `REGISTRY_PASSWORD`,
  `KUBE_CONFIG_DATA_K3S`, `RP_API_TOKEN`.

## 6. Follow-ups / TODO
- [ ] Review + merge the agent-platform stack (#18→#19→#20) and redesign (#21).
- [ ] Install **pgvector** on the k3s1 Postgres before Phase 2 (#19) is deployed.
- [ ] Register test/acc/prod public hosts on the wslproxy edge (like int) if public
      access to those rings is wanted.
- [ ] Phase 4 (triggers) + Phase 5 (evaluator guardrails); native-path approval
      pause; approvals-queue UI; `agent_approval_rules` admin endpoint.
- [ ] Per-screen redesign polish (Signal Room).
- [ ] **Security housekeeping:** `rm /tmp/wsl-creds` (wslproxy creds on disk);
      `RP_API_TOKEN` was copied from the cluster into a jobshout GitHub secret —
      rotate if you'd prefer.
- [ ] Consider ArgoCD (installed on k3s1) as an alternative GitOps path — currently
      unused for jobshout; RP is the chosen mechanism.
