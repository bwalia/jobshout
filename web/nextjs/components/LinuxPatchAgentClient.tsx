"use client";

import { useEffect, useState } from "react";
import { Check, Loader2, Minus, X } from "lucide-react";
import { toast } from "sonner";
import { apiClient, apiErrorMessage } from "@/lib/api/client";
import {
  cancelLinuxPatchRun,
  createLinuxPatchRun,
  getLinuxPatchRun,
  listLinuxPatchRuns,
  linuxPatchStatus,
  type LinuxPatchRun,
} from "@/lib/api/linux-patch";

const inputClass =
  "mt-1 w-full rounded-md border border-input bg-background px-3 py-2 text-sm focus:outline-none focus-visible:ring-2 focus-visible:ring-ring";

function PhaseIcon({ status }: { status?: string }) {
  if (status === "completed") return <Check className="h-4 w-4 text-emerald-600" strokeWidth={2.5} />;
  if (status === "failed") return <X className="h-4 w-4 text-destructive" strokeWidth={2.5} />;
  if (status === "skipped") return <Minus className="h-4 w-4 text-muted-foreground" strokeWidth={2.5} />;
  if (status === "active") return <Loader2 className="h-4 w-4 animate-spin text-primary" />;
  return <span className="block h-2 w-2 rounded-full bg-muted-foreground/40" />;
}

export function LinuxPatchAgentClient() {
  const [agentId, setAgentId] = useState("");
  const [statusLine, setStatusLine] = useState("");
  const [selected, setSelected] = useState<LinuxPatchRun | null>(null);
  const [history, setHistory] = useState<LinuxPatchRun[]>([]);
  const [tab, setTab] = useState<"run" | "history">("run");

  const [mode, setMode] = useState("plan");
  const [workload, setWorkload] = useState("wslvault");
  const [hosts, setHosts] = useState("");
  const [vaultPath, setVaultPath] = useState("ops/ssh/patch-fleet");
  const [vaultMount, setVaultMount] = useState("secret");
  const [services, setServices] = useState("");
  const [dryRun, setDryRun] = useState("true");
  const [useLlm, setUseLlm] = useState("true");
  const [instruction, setInstruction] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  useEffect(() => {
    void (async () => {
      try {
        const [{ data }, st] = await Promise.all([
          apiClient.get<{ data: Array<{ id: string; metadata?: { builtin?: string } }> }>("/agents", {
            params: { per_page: 100 },
          }),
          linuxPatchStatus().catch(() => null),
        ]);
        const agent = (data.data ?? []).find((a) => a.metadata?.builtin === "linux_patch");
        if (agent) setAgentId(agent.id);
        if (st) setStatusLine(String(st.message || ""));
      } catch {
        setError("Failed to load Linux Patch agent");
      }
    })();
  }, []);

  useEffect(() => {
    if (!selected || ["completed", "failed", "cancelled"].includes(selected.status)) return;
    const id = window.setInterval(() => {
      void getLinuxPatchRun(selected.id).then(setSelected).catch(() => undefined);
    }, 2000);
    return () => window.clearInterval(id);
  }, [selected]);

  useEffect(() => {
    if (tab !== "history") return;
    void listLinuxPatchRuns(1, 20)
      .then((p) => setHistory(p.data ?? []))
      .catch((e) => toast.error(apiErrorMessage(e, "Failed to load runs")));
  }, [tab]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!agentId) return;
    setBusy(true);
    setError("");
    try {
      const run = await createLinuxPatchRun({
        agent_id: agentId,
        mode,
        workload,
        hosts,
        vault_path: vaultPath.trim() || undefined,
        vault_mount: vaultMount.trim() || "secret",
        services: services.trim() || undefined,
        dry_run: dryRun === "true",
        use_llm: useLlm === "true",
        instruction: instruction.trim() || undefined,
      });
      setSelected(run);
      toast.success("Linux Patch run queued");
    } catch (err) {
      setError(apiErrorMessage(err, "Failed to start"));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="space-y-4 p-4">
      <header className="space-y-2">
        <h2 className="text-lg font-semibold">Linux Patch</h2>
        <p className="text-sm text-muted-foreground">
          Rolling SSH patching with LLM planning. Pull SSH keys from{" "}
          <a className="underline" href="https://www.wslvault.org/" target="_blank" rel="noreferrer">
            WSLVault
          </a>{" "}
          / HashiCorp Vault, then chain via workflow <em>Vault SSH then Linux Patch</em> (Secrets
          Rotation → this agent).
        </p>
        {statusLine && (
          <p className="rounded-md border bg-muted/40 px-3 py-2 text-xs text-muted-foreground">
            {statusLine}
          </p>
        )}
      </header>

      <div className="flex gap-2 border-b pb-2">
        {(["run", "history"] as const).map((t) => (
          <button
            key={t}
            type="button"
            onClick={() => setTab(t)}
            className={`rounded-md px-3 py-1.5 text-sm capitalize ${
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
              <label className="text-sm font-medium">
                Mode
                <select className={inputClass} value={mode} onChange={(e) => setMode(e.target.value)}>
                  <option value="plan">Plan</option>
                  <option value="patch">Patch</option>
                  <option value="verify">Verify</option>
                </select>
              </label>
              <label className="text-sm font-medium">
                Workload
                <select
                  className={inputClass}
                  value={workload}
                  onChange={(e) => setWorkload(e.target.value)}
                >
                  <option value="wslvault">WSLVault (HA)</option>
                  <option value="hashicorp_vault">HashiCorp Vault</option>
                  <option value="couchbase">Couchbase</option>
                  <option value="generic">Generic</option>
                </select>
              </label>
            </div>
            <label className="text-sm font-medium">
              Hosts
              <textarea
                className={inputClass}
                rows={3}
                required
                value={hosts}
                onChange={(e) => setHosts(e.target.value)}
                placeholder={"user@host1\nuser@host2"}
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="text-sm font-medium">
                Vault path (SSH key)
                <input
                  className={inputClass}
                  value={vaultPath}
                  onChange={(e) => setVaultPath(e.target.value)}
                />
              </label>
              <label className="text-sm font-medium">
                Vault mount
                <input
                  className={inputClass}
                  value={vaultMount}
                  onChange={(e) => setVaultMount(e.target.value)}
                />
              </label>
            </div>
            <label className="text-sm font-medium">
              Services to ensure
              <input
                className={inputClass}
                value={services}
                onChange={(e) => setServices(e.target.value)}
                placeholder="wslvault, nginx"
              />
            </label>
            <div className="grid gap-3 sm:grid-cols-2">
              <label className="text-sm font-medium">
                Dry run
                <select className={inputClass} value={dryRun} onChange={(e) => setDryRun(e.target.value)}>
                  <option value="true">Yes (safe)</option>
                  <option value="false">Live</option>
                </select>
              </label>
              <label className="text-sm font-medium">
                LLM planning
                <select className={inputClass} value={useLlm} onChange={(e) => setUseLlm(e.target.value)}>
                  <option value="true">On</option>
                  <option value="false">Off</option>
                </select>
              </label>
            </div>
            <label className="text-sm font-medium">
              Note
              <textarea
                className={inputClass}
                rows={2}
                value={instruction}
                onChange={(e) => setInstruction(e.target.value)}
              />
            </label>
            {error && <p className="text-sm text-destructive">{error}</p>}
            <button
              type="submit"
              disabled={!agentId || busy}
              className="rounded-md bg-primary px-4 py-2 text-sm text-primary-foreground disabled:opacity-50"
            >
              {busy ? "Starting…" : "Start"}
            </button>
          </form>

          <div>
            {!selected && (
              <p className="text-sm text-muted-foreground">
                Prefer Plan + dry-run. Use workflow <strong>Vault SSH then Linux Patch</strong> to
                orchestrate Secrets Rotation first.
              </p>
            )}
            {selected && (
              <div className="space-y-3 rounded-xl border p-4">
                <div className="flex justify-between gap-2">
                  <div>
                    <p className="text-sm font-medium">
                      {selected.mode} · {selected.workload} · {selected.status}
                    </p>
                    <p className="text-xs text-muted-foreground">{selected.hosts}</p>
                  </div>
                  {(selected.status === "queued" || selected.status === "running") && (
                    <button
                      type="button"
                      className="text-xs underline"
                      onClick={() =>
                        void cancelLinuxPatchRun(selected.id)
                          .then(setSelected)
                          .catch((e) => toast.error(apiErrorMessage(e, "Cancel failed")))
                      }
                    >
                      Cancel
                    </button>
                  )}
                </div>
                {selected.error_message && (
                  <p className="text-sm text-destructive">{selected.error_message}</p>
                )}
                <ul className="space-y-2">
                  {(selected.phases ?? []).map((p) => (
                    <li key={p.key} className="flex gap-2 text-sm">
                      <PhaseIcon status={p.status} />
                      <div>
                        <p className="font-medium">{p.label}</p>
                        {p.message && <p className="text-xs text-muted-foreground">{p.message}</p>}
                      </div>
                    </li>
                  ))}
                </ul>
                {selected.plan && (
                  <div className="rounded-md border bg-muted/30 p-3 text-sm">
                    <p>{selected.plan.strategy}</p>
                    {selected.plan.llm_summary && (
                      <p className="mt-1 text-muted-foreground">{selected.plan.llm_summary}</p>
                    )}
                    <ol className="mt-2 list-decimal space-y-1 pl-4 text-xs text-muted-foreground">
                      {(selected.plan.steps ?? []).map((s) => (
                        <li key={s}>{s}</li>
                      ))}
                    </ol>
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {tab === "history" && (
        <ul className="divide-y rounded-md border">
          {history.map((r) => (
            <li key={r.id}>
              <button
                type="button"
                className="flex w-full justify-between px-3 py-2 text-left text-sm hover:bg-muted/50"
                onClick={() => {
                  setSelected(r);
                  setTab("run");
                }}
              >
                <span className="truncate">{r.workload}</span>
                <span className="text-xs text-muted-foreground">
                  {r.mode} · {r.status}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
