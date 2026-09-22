"use client";

import { useEffect, useState } from "react";
import { Check, Loader2, Minus, X } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { toast } from "sonner";
import { apiClient, apiErrorMessage } from "@/lib/api/client";
import {
  cancelSecretsRotationRun,
  createSecretsRotationRun,
  getSecretsRotationRun,
  listSecretsRotationRuns,
  secretsRotationStatus,
  type SecretsRotationPhase,
  type SecretsRotationPropagation,
  type SecretsRotationRun,
} from "@/lib/api/secrets-rotation";

type Tab = "run" | "history";

const inputClass =
  "mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50";

interface AgentSummary {
  id: string;
  metadata?: { builtin?: string };
}

function PhaseIcon({ status }: { status?: string }) {
  if (status === "completed") return <Check className="h-4 w-4 text-emerald-600" strokeWidth={2.5} />;
  if (status === "failed") return <X className="h-4 w-4 text-destructive" strokeWidth={2.5} />;
  if (status === "skipped") return <Minus className="h-4 w-4 text-muted-foreground" strokeWidth={2.5} />;
  if (status === "active") return <Loader2 className="h-4 w-4 animate-spin text-primary" />;
  return <span className="block h-2 w-2 rounded-full bg-muted-foreground/40" />;
}

export function SecretsRotationAgentClient() {
  const searchParams = useSearchParams();
  const runParam = searchParams.get("run");

  const [agentId, setAgentId] = useState("");
  const [loadError, setLoadError] = useState("");
  const [statusLine, setStatusLine] = useState("");
  const [tab, setTab] = useState<Tab>("run");
  const [selected, setSelected] = useState<SecretsRotationRun | null>(null);
  const [history, setHistory] = useState<SecretsRotationRun[]>([]);

  const [mode, setMode] = useState("plan");
  const [provider, setProvider] = useState("auto");
  const [vaultAddr, setVaultAddr] = useState("https://vault.workstation.co.uk");
  const [engine, setEngine] = useState("kv2");
  const [mount, setMount] = useState("secret");
  const [path, setPath] = useState("");
  const [keys, setKeys] = useState("");
  const [grace, setGrace] = useState("300");
  const [retireOld, setRetireOld] = useState("false");
  const [dryRun, setDryRun] = useState("false");
  const [instruction, setInstruction] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [formError, setFormError] = useState("");

  useEffect(() => {
    void (async () => {
      try {
        const [{ data }, st] = await Promise.all([
          apiClient.get<{ data: AgentSummary[] }>("/agents", { params: { per_page: 100 } }),
          secretsRotationStatus().catch(() => null),
        ]);
        const agents = Array.isArray(data.data) ? data.data : [];
        const agent = agents.find((a) => a.metadata?.builtin === "secrets_rotation");
        if (!agent) {
          setLoadError("No Secrets Rotation agent found for this organization.");
          return;
        }
        setAgentId(agent.id);
        if (st) {
          if (typeof st.vault_addr === "string" && st.vault_addr) {
            setVaultAddr(st.vault_addr);
          }
          setStatusLine(String(st.message || ""));
        }
      } catch {
        setLoadError("Failed to load the Secrets Rotation agent.");
      }
    })();
  }, []);

  useEffect(() => {
    if (!runParam) return;
    let cancelled = false;
    void (async () => {
      try {
        const run = await getSecretsRotationRun(runParam);
        if (!cancelled) {
          setSelected(run);
          setTab("run");
        }
      } catch {
        // History may still list it.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runParam]);

  useEffect(() => {
    if (
      !selected ||
      selected.status === "completed" ||
      selected.status === "failed" ||
      selected.status === "cancelled"
    ) {
      return;
    }
    let cancelled = false;
    const id = window.setInterval(() => {
      void (async () => {
        try {
          const run = await getSecretsRotationRun(selected.id);
          if (!cancelled) setSelected(run);
        } catch {
          /* ignore transient */
        }
      })();
    }, 1500);
    return () => {
      cancelled = true;
      window.clearInterval(id);
    };
  }, [selected]);

  useEffect(() => {
    if (tab !== "history") return;
    let cancelled = false;
    void (async () => {
      try {
        const page = await listSecretsRotationRuns(1, 20);
        if (!cancelled) setHistory(page.data ?? []);
      } catch (e) {
        toast.error(apiErrorMessage(e, "Failed to load runs"));
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [tab]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!agentId) return;
    setFormError("");
    setSubmitting(true);
    try {
      const run = await createSecretsRotationRun({
        agent_id: agentId,
        mode,
        provider,
        vault_addr: vaultAddr.trim() || undefined,
        mount: mount.trim() || "secret",
        path: path.trim(),
        engine,
        keys: keys.trim() || undefined,
        grace_seconds: Number.parseInt(grace, 10) || 0,
        retire_old: retireOld === "true",
        dry_run: dryRun === "true",
        instruction: instruction.trim() || undefined,
      });
      setSelected(run);
      toast.success("Secrets rotation run queued");
    } catch (err) {
      setFormError(apiErrorMessage(err, "Failed to start run"));
    } finally {
      setSubmitting(false);
    }
  };

  if (loadError) {
    return (
      <div className="p-4">
        <p className="text-sm text-destructive">{loadError}</p>
      </div>
    );
  }

  return (
    <div className="space-y-4 p-4">
      <header className="space-y-2">
        <h2 className="text-lg font-semibold">Secrets Rotation</h2>
        <p className="text-sm text-muted-foreground">
          Zero-downtime rotation for{" "}
          <a
            className="underline underline-offset-2"
            href="https://www.wslvault.org/"
            target="_blank"
            rel="noreferrer"
          >
            WSLVault
          </a>{" "}
          and HashiCorp Vault — KV v2 dual-version windows, transit key rotate, DB lease cutover
          plans. Console:{" "}
          <a
            className="underline underline-offset-2"
            href="https://vault-ui.workstation.co.uk/login"
            target="_blank"
            rel="noreferrer"
          >
            vault-ui.workstation.co.uk
          </a>
          .
        </p>
        {statusLine && (
          <p className="rounded-md border border-border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            {statusLine}
          </p>
        )}
      </header>

      <div className="flex gap-2 border-b border-border pb-2">
        {(["run", "history"] as Tab[]).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={`rounded-md px-3 py-1.5 text-sm font-medium capitalize ${
              tab === t ? "bg-primary text-primary-foreground" : "hover:bg-muted"
            }`}
          >
            {t}
          </button>
        ))}
      </div>

      {tab === "run" && (
        <div className="grid gap-6 lg:grid-cols-2">
          <form onSubmit={submit} className="space-y-3">
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm font-medium">
                Mode
                <select className={inputClass} value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="plan">Plan only</option>
                  <option value="rotate">Rotate (zero downtime)</option>
                  <option value="verify">Verify</option>
                  <option value="rollback">Rollback</option>
                </select>
              </label>
              <label className="block text-sm font-medium">
                Provider
                <select
                  className={inputClass}
                  value={provider}
                  onChange={(e) => setProvider(e.target.value)}
                >
                  <option value="auto">Auto-detect</option>
                  <option value="wslvault">WSLVault</option>
                  <option value="hashicorp">HashiCorp Vault</option>
                </select>
              </label>
            </div>
            <label className="block text-sm font-medium">
              Vault API address
              <input
                className={inputClass}
                value={vaultAddr}
                onChange={(e) => setVaultAddr(e.target.value)}
                placeholder="https://vault.workstation.co.uk"
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="block text-sm font-medium">
                Engine
                <select
                  className={inputClass}
                  value={engine}
                  onChange={(e) => setEngine(e.target.value)}
                >
                  <option value="kv2">KV v2</option>
                  <option value="transit">Transit</option>
                  <option value="database">Database (plan)</option>
                </select>
              </label>
              <label className="block text-sm font-medium">
                Mount
                <input
                  className={inputClass}
                  value={mount}
                  onChange={(e) => setMount(e.target.value)}
                />
              </label>
            </div>
            <label className="block text-sm font-medium">
              Secret path
              <input
                className={inputClass}
                value={path}
                onChange={(e) => setPath(e.target.value)}
                placeholder="prod/db/creds"
                required
              />
            </label>
            <label className="block text-sm font-medium">
              Keys to rotate
              <input
                className={inputClass}
                value={keys}
                onChange={(e) => setKeys(e.target.value)}
                placeholder="password, api_key"
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-3">
              <label className="block text-sm font-medium">
                Dual-window (s)
                <input
                  className={inputClass}
                  value={grace}
                  onChange={(e) => setGrace(e.target.value)}
                />
              </label>
              <label className="block text-sm font-medium">
                Retire old
                <select
                  className={inputClass}
                  value={retireOld}
                  onChange={(e) => setRetireOld(e.target.value)}
                >
                  <option value="false">Keep</option>
                  <option value="true">Soft-delete</option>
                </select>
              </label>
              <label className="block text-sm font-medium">
                Dry run
                <select
                  className={inputClass}
                  value={dryRun}
                  onChange={(e) => setDryRun(e.target.value)}
                >
                  <option value="false">Live</option>
                  <option value="true">Dry run</option>
                </select>
              </label>
            </div>
            <label className="block text-sm font-medium">
              Note
              <textarea
                className={inputClass}
                rows={2}
                value={instruction}
                onChange={(e) => setInstruction(e.target.value)}
              />
            </label>
            {formError && <p className="text-sm text-destructive">{formError}</p>}
            <button
              type="submit"
              disabled={!agentId || submitting}
              className="rounded-md bg-primary px-4 py-2 text-sm font-medium text-primary-foreground disabled:opacity-50"
            >
              {submitting ? "Starting…" : "Start"}
            </button>
          </form>

          <div className="space-y-3">
            {!selected && (
              <p className="text-sm text-muted-foreground">
                Prefer <strong>Plan</strong> first, then <strong>Rotate</strong> with a dual-read
                window so consumers never see a missing secret. To also propagate to Kubernetes,
                GitHub Actions or restart rings, launch from New task or chat (Propagate fields).
              </p>
            )}
            {selected && <RunDetail run={selected} onCancel={canCancel(selected) ? async () => {
              try {
                setSelected(await cancelSecretsRotationRun(selected.id));
                toast.success("Cancelled");
              } catch (e) {
                toast.error(apiErrorMessage(e, "Cancel failed"));
              }
            } : undefined} />}
          </div>
        </div>
      )}

      {tab === "history" && (
        <ul className="divide-y divide-border rounded-md border border-border">
          {history.length === 0 && (
            <li className="px-3 py-4 text-sm text-muted-foreground">No runs yet.</li>
          )}
          {history.map((r) => (
            <li key={r.id}>
              <button
                type="button"
                className="flex w-full items-center justify-between gap-3 px-3 py-2.5 text-left text-sm hover:bg-muted/50"
                onClick={() => {
                  setSelected(r);
                  setTab("run");
                }}
              >
                <span className="truncate font-medium">
                  {r.mount}/{r.path}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground">
                  {r.mode} · {r.status}
                  {r.result?.current_version != null ? ` · v${r.result.current_version}` : ""}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function canCancel(run: SecretsRotationRun) {
  return run.status === "queued" || run.status === "running";
}

function RunDetail({
  run,
  onCancel,
}: {
  run: SecretsRotationRun;
  onCancel?: () => void;
}) {
  const phases: SecretsRotationPhase[] = run.phases ?? [];
  const result = run.result;

  return (
    <div className="space-y-4 rounded-xl border border-border p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0 space-y-1">
          <p className="truncate text-sm font-medium">
            {run.mount}/{run.path}
          </p>
          <p className="text-xs text-muted-foreground">
            {run.mode} · {run.status}
            {run.detected_provider ? ` · ${run.detected_provider}` : ""}
            {run.dry_run ? " · dry-run" : ""}
          </p>
        </div>
        {onCancel && (
          <button
            type="button"
            onClick={onCancel}
            className="rounded-md border border-border px-2 py-1 text-xs hover:bg-muted"
          >
            Cancel
          </button>
        )}
      </div>

      {run.error_message && <p className="text-sm text-destructive">{run.error_message}</p>}

      {phases.length > 0 && (
        <ol className="space-y-2">
          {phases.map((p) => (
            <li key={p.key} className="flex gap-3 text-sm">
              <PhaseIcon status={p.status} />
              <div className="min-w-0">
                <p className="font-medium">{p.label}</p>
                {p.message && <p className="text-xs text-muted-foreground">{p.message}</p>}
              </div>
            </li>
          ))}
        </ol>
      )}

      {result && (
        <section className="space-y-2 rounded-md border border-border bg-muted/30 px-3 py-2 text-sm">
          {result.strategy && <p className="text-muted-foreground">{result.strategy}</p>}
          <dl className="grid gap-1 sm:grid-cols-2">
            {result.previous_version != null && (
              <div>
                <dt className="text-xs text-muted-foreground">Previous</dt>
                <dd>v{result.previous_version}</dd>
              </div>
            )}
            {result.current_version != null && (
              <div>
                <dt className="text-xs text-muted-foreground">Current</dt>
                <dd>v{result.current_version}</dd>
              </div>
            )}
            {result.rolled_back_to != null && (
              <div>
                <dt className="text-xs text-muted-foreground">Rolled back to</dt>
                <dd>v{result.rolled_back_to}</dd>
              </div>
            )}
            {result.keys_rotated && result.keys_rotated.length > 0 && (
              <div className="sm:col-span-2">
                <dt className="text-xs text-muted-foreground">Keys</dt>
                <dd className="font-mono text-xs">{result.keys_rotated.join(", ")}</dd>
              </div>
            )}
          </dl>
          {result.propagation && <PropagationSummary propagation={result.propagation} />}
          {result.plan_steps && result.plan_steps.length > 0 && (
            <ol className="list-decimal space-y-1 pl-4 text-xs text-muted-foreground">
              {result.plan_steps.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
          )}
          {result.warnings && result.warnings.length > 0 && (
            <ul className="space-y-1 text-xs text-amber-800 dark:text-amber-200">
              {result.warnings.map((w) => (
                <li key={w}>{w}</li>
              ))}
            </ul>
          )}
          <div className="flex flex-wrap gap-3 pt-1 text-xs">
            {result.ui_login_url && (
              <a
                className="underline underline-offset-2"
                href={result.ui_login_url}
                target="_blank"
                rel="noreferrer"
              >
                Open vault UI
              </a>
            )}
            {result.docs_url && (
              <a
                className="underline underline-offset-2"
                href={result.docs_url}
                target="_blank"
                rel="noreferrer"
              >
                WSLVault docs
              </a>
            )}
          </div>
        </section>
      )}
    </div>
  );
}

const TARGET_KIND_LABEL: Record<string, string> = {
  k8s_secret: "Secret",
  external_secret: "ExternalSecret",
  github_secret: "GitHub",
  ring: "Ring",
};

function PropagationSummary({ propagation }: { propagation: SecretsRotationPropagation }) {
  const targets = propagation.targets ?? [];
  return (
    <div className="space-y-1">
      <p className="text-xs text-muted-foreground">
        Propagation · source {propagation.source}
        {propagation.cluster ? ` · cluster ${propagation.cluster}` : ""}
        {propagation.vault_version ? ` · Vault v${propagation.vault_version}` : ""}
      </p>
      {targets.length > 0 && (
        <ul className="space-y-1 text-xs">
          {targets.map((t) => (
            <li key={`${t.kind}:${t.name}`} className="flex gap-2">
              <PhaseIcon status={t.status === "dry_run" ? "skipped" : t.status} />
              <div className="min-w-0">
                <p>
                  <span className="text-muted-foreground">{TARGET_KIND_LABEL[t.kind] ?? t.kind}</span>{" "}
                  <span className="font-mono">{t.name}</span>
                  {t.status === "dry_run" ? " · dry-run" : ""}
                  {t.resource_version ? ` · rv ${t.resource_version}` : ""}
                  {t.job_id ? ` · job ${t.job_id}` : ""}
                </p>
                {t.message && <p className="text-muted-foreground">{t.message}</p>}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
