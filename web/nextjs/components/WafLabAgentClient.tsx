"use client";

import { useEffect, useState } from "react";
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
              <p className="text-sm text-muted-foreground">
                {selected.status} · {selected.mode} · {selected.attack_set}
              </p>
              {selected.error_message && (
                <p className="mt-2 text-sm text-destructive">{selected.error_message}</p>
              )}
            </div>
            {detailError && <p className="text-sm text-destructive">{detailError}</p>}
            {steps.length > 0 && (
              <ol className="space-y-1 text-sm">
                {steps.map((s) => (
                  <li key={s.id} className="flex gap-2">
                    <span className="w-24 shrink-0 font-medium capitalize text-foreground">{s.phase}</span>
                    <span className="text-muted-foreground">
                      {s.status}: {s.message}
                    </span>
                  </li>
                ))}
              </ol>
            )}
            <WafLabMatrix
              results={results}
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
