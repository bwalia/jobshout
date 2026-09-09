"use client";

import { useEffect, useState } from "react";
import { Check, Loader2, Minus, X } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { apiClient, apiErrorMessage } from "@/lib/api/client";
import {
  cancelWafLabRun,
  getWafLabRun,
  listWafLabResults,
  listWafLabSteps,
  wafLabStatus,
  type WAFLabResult,
  type WAFLabRun,
  type WAFLabStep,
} from "@/lib/api/waf-lab";
import { WafLabMatrix } from "@/components/waflab/WafLabMatrix";
import { WafLabRunForm } from "@/components/waflab/WafLabRunForm";
import { WafLabRunsList } from "@/components/waflab/WafLabRunsList";

type Tab = "run" | "history";

const PHASES: { key: string; label: string }[] = [
  { key: "preflight", label: "Preflight" },
  { key: "rules_policy", label: "Rules & policy" },
  { key: "hosts", label: "Hosts" },
  { key: "dns", label: "DNS" },
  { key: "efficacy", label: "Attack matrix" },
];

function PhaseIcon({ status }: { status?: string }) {
  if (status === "completed") return <Check className="h-4 w-4 text-signal" strokeWidth={2.5} />;
  if (status === "failed") return <X className="h-4 w-4 text-destructive" strokeWidth={2.5} />;
  if (status === "skipped") return <Minus className="h-4 w-4 text-muted-foreground" strokeWidth={2.5} />;
  if (status === "active") return <Loader2 className="h-4 w-4 animate-spin text-primary" />;
  return <span className="block h-2 w-2 rounded-full bg-muted-foreground/40" />;
}

interface AgentSummary {
  id: string;
  metadata?: { builtin?: string };
}

export function WafLabAgentClient() {
  const searchParams = useSearchParams();
  const runParam = searchParams.get("run");

  const [agentId, setAgentId] = useState("");
  const [loadError, setLoadError] = useState("");
  const [statusLine, setStatusLine] = useState("");
  const [selected, setSelected] = useState<WAFLabRun | null>(null);
  const [steps, setSteps] = useState<WAFLabStep[]>([]);
  const [results, setResults] = useState<WAFLabResult[]>([]);
  const [tab, setTab] = useState<Tab>("run");
  const [detailError, setDetailError] = useState("");

  useEffect(() => {
    void (async () => {
      try {
        const [{ data }, st] = await Promise.all([
          apiClient.get<{ data: AgentSummary[] }>("/agents", { params: { per_page: 100 } }),
          wafLabStatus().catch(() => null),
        ]);
        const agents = Array.isArray(data.data) ? data.data : [];
        const agent = agents.find((a) => a.metadata?.builtin === "waf_lab");
        if (!agent) {
          setLoadError("No WAF Efficacy Lab agent found for this organization.");
          return;
        }
        setAgentId(agent.id);
        if (st) {
          setStatusLine(
            st.enabled
              ? `Connected to ${String(st.wslproxy_base_url || "wslproxy")}`
              : "Not configured — set WSLPROXY_BASE_URL and credentials on the API."
          );
        }
      } catch {
        setLoadError("Failed to load the WAF Efficacy Lab agent.");
      }
    })();
  }, []);

  useEffect(() => {
    if (!runParam) return;
    let cancelled = false;
    void (async () => {
      try {
        const run = await getWafLabRun(runParam);
        if (cancelled) return;
        setSelected(run);
        setTab("run");
      } catch {
        // Leave the form; run may still appear under History.
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [runParam]);

  useEffect(() => {
    if (!selected) {
      setSteps([]);
      setResults([]);
      return;
    }
    let cancelled = false;
    const load = async () => {
      try {
        const [run, st, res] = await Promise.all([
          getWafLabRun(selected.id),
          listWafLabSteps(selected.id),
          listWafLabResults(selected.id),
        ]);
        if (cancelled) return;
        setSelected(run);
        setSteps(st);
        setResults(res);
        setDetailError("");
      } catch (err) {
        if (!cancelled) setDetailError(apiErrorMessage(err, "Failed to load run detail"));
      }
    };
    void load();
    const active = selected.status === "queued" || selected.status === "running";
    if (!active) return;
    const t = window.setInterval(() => void load(), 2500);
    return () => {
      cancelled = true;
      window.clearInterval(t);
    };
  }, [selected?.id, selected?.status]);

  if (loadError) {
    return (
      <div className="rounded-lg border border-destructive/40 bg-destructive/10 p-6 text-center text-sm text-destructive">
        {loadError}
      </div>
    );
  }

  if (!agentId) {
    return (
      <div className="rounded-lg border border-border bg-card p-6 text-center text-muted-foreground">
        Loading agent configuration…
      </div>
    );
  }

  const runActive = selected?.status === "queued" || selected?.status === "running";
  const stepByPhase = new Map(steps.map((s) => [s.phase, s]));
  const activePhase = PHASES.find((p) => !stepByPhase.has(p.key))?.key ?? "";
  const activeLabel =
    PHASES.find((p) => p.key === activePhase)?.label.toLowerCase() ?? "finishing up";

  const tabClass = (t: Tab) =>
    `flex-1 rounded-md px-3 py-1.5 text-sm font-medium ${
      tab === t ? "bg-card text-card-foreground shadow" : "text-muted-foreground hover:text-foreground"
    }`;

  return (
    <div className="space-y-4">
      {statusLine && <p className="text-sm text-muted-foreground">{statusLine}</p>}

      <div className="flex gap-1 rounded-lg bg-muted p-1">
        <button type="button" className={tabClass("run")} onClick={() => setTab("run")}>
          Lab run
        </button>
        <button type="button" className={tabClass("history")} onClick={() => setTab("history")}>
          History
        </button>
      </div>

      {tab === "run" &&
        (selected ? (
          <div className="space-y-4 rounded-lg border border-border bg-card p-4 text-card-foreground">
            <div className="flex flex-wrap items-center justify-between gap-2">
              <button
                type="button"
                onClick={() => setSelected(null)}
                className="text-sm font-medium text-primary hover:underline"
              >
                ← Back to form
              </button>
              {(selected.status === "queued" || selected.status === "running") && (
                <button
                  type="button"
                  className="text-sm text-destructive hover:underline"
                  onClick={() => void cancelWafLabRun(selected.id).then(setSelected)}
                >
                  Cancel
                </button>
              )}
            </div>
            <div>
              <h2 className="font-semibold">{selected.secure_host}</h2>
              <p className="flex flex-wrap items-center gap-2 text-sm text-muted-foreground">
                {runActive && <Loader2 className="h-4 w-4 animate-spin text-primary" />}
                <span className={runActive ? "font-medium text-foreground" : undefined}>
                  {runActive ? `Running — ${activeLabel}` : selected.status}
                </span>
                <span>· {selected.mode} · {selected.attack_set}</span>
              </p>
              {runActive && (
                <p className="mt-1 text-sm text-muted-foreground">
                  This runs on wslproxy and takes a few minutes. The page updates itself —
                  you can leave it open or come back to it from History.
                </p>
              )}
              {selected.error_message && (
                <p className="mt-2 text-sm text-destructive">{selected.error_message}</p>
              )}
            </div>
            {detailError && <p className="text-sm text-destructive">{detailError}</p>}
            <ol className="space-y-2 text-sm">
              {PHASES.map((phase) => {
                const step = stepByPhase.get(phase.key);
                const isActive = runActive && phase.key === activePhase;
                return (
                  <li key={phase.key} className="flex gap-3">
                    <span className="mt-0.5 flex h-4 w-4 shrink-0 items-center justify-center">
                      <PhaseIcon status={isActive ? "active" : step?.status} />
                    </span>
                    <span className="min-w-0 flex-1">
                      <span
                        className={
                          step || isActive ? "font-medium text-foreground" : "text-muted-foreground"
                        }
                      >
                        {phase.label}
                      </span>
                      <span className="block break-words text-muted-foreground">
                        {step
                          ? `${step.status}: ${step.message}`
                          : isActive
                            ? "Working…"
                            : "Waiting"}
                      </span>
                    </span>
                  </li>
                );
              })}
            </ol>
            <WafLabMatrix
              results={results}
              running={runActive}
              score={selected.score}
              secureHost={selected.secure_host}
              openHost={selected.open_host}
            />
          </div>
        ) : (
          <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
            <h2 className="font-semibold">Start WAF Efficacy Lab</h2>
            <p className="mb-4 text-sm text-muted-foreground">
              Provision a secure/open pair on wslproxy and measure the attack matrix.
            </p>
            <WafLabRunForm agentId={agentId} onRunCreated={setSelected} />
          </div>
        ))}

      {tab === "history" && (
        <div className="rounded-lg border border-border bg-card p-4 text-card-foreground">
          <WafLabRunsList
            onRunSelected={(run) => {
              setSelected(run);
              setTab("run");
            }}
          />
        </div>
      )}
    </div>
  );
}
