"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { apiErrorMessage } from "@/lib/api/client";
import {
  abTestStatus,
  listABExperiments,
  observeAB,
  promoteAB,
  rollbackAB,
  setABWeights,
  type ABExperiment,
  type ABObserveResult,
} from "@/lib/api/ab-testing";

const btn =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 border border-border bg-background hover:bg-muted";
const btnPrimary =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 bg-primary text-primary-foreground hover:bg-primary/90";

export function AbTestAgentClient() {
  const [statusLine, setStatusLine] = useState("");
  const [experiments, setExperiments] = useState<ABExperiment[]>([]);
  const [selected, setSelected] = useState<ABExperiment | null>(null);
  const [stable, setStable] = useState(80);
  const [canary, setCanary] = useState(20);
  const [observe, setObserve] = useState<ABObserveResult | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    try {
      setError("");
      const [st, list] = await Promise.all([abTestStatus(), listABExperiments()]);
      setStatusLine(
        st.mcp_configured
          ? `Live MCP · ${String(st.message ?? "")}`
          : `Demo mode · ${String(st.message ?? "")}`
      );
      setExperiments(list.experiments ?? []);
      const first = list.experiments?.[0] ?? null;
      setSelected((prev) => {
        if (prev && list.experiments?.some((e) => e.id === prev.id)) {
          return list.experiments.find((e) => e.id === prev.id) ?? first;
        }
        return first;
      });
      if (first?.backends?.length) {
        const v1 = first.backends.find((b) => b.label === "v1" || b.role === "stable");
        const v2 = first.backends.find((b) => b.label === "v2" || b.role === "canary");
        if (v1) setStable(v1.weight);
        if (v2) setCanary(v2.weight);
      }
    } catch (e) {
      setError(apiErrorMessage(e, "Failed to load AB Testing agent"));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  async function run(label: string, fn: () => Promise<void>) {
    setBusy(label);
    try {
      await fn();
    } catch (e) {
      toast.error(apiErrorMessage(e, "Request failed"));
    } finally {
      setBusy(null);
    }
  }

  const id = selected?.id ?? "abtesting";

  return (
    <div className="space-y-4 p-4">
      <header className="space-y-1">
        <h2 className="text-lg font-semibold">AB Testing</h2>
        <p className="text-sm text-muted-foreground">
          Weighted / canary traffic splits on wslproxy — control plane that replaces the
          standalone{" "}
          <a
            className="underline underline-offset-2"
            href="https://abtesting.fictionally.org/"
            target="_blank"
            rel="noreferrer"
          >
            abtesting.fictionally.org
          </a>{" "}
          demo UI. Mutations go through the wslproxy MCP server.
        </p>
        {statusLine && <p className="text-xs text-muted-foreground">{statusLine}</p>}
        {error && <p className="text-sm text-destructive">{error}</p>}
      </header>

      {selected && (
        <section className="rounded-xl border p-4 space-y-3">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <div>
              <h3 className="font-semibold">{selected.name}</h3>
              <p className="text-xs text-muted-foreground">
                {selected.host} · rule {selected.rule_id} · {selected.mode}
              </p>
            </div>
            <a
              className="text-sm underline underline-offset-2"
              href={selected.public_url}
              target="_blank"
              rel="noreferrer"
            >
              Open public host
            </a>
          </div>
          {selected.note && <p className="text-sm text-muted-foreground">{selected.note}</p>}

          <div className="flex flex-wrap items-end gap-3">
            <label className="text-sm">
              <span className="text-muted-foreground">v1 / stable %</span>
              <input
                type="number"
                min={0}
                max={100}
                className="mt-1 block w-24 rounded-md border bg-background px-2 py-1"
                value={stable}
                onChange={(e) => setStable(Number(e.target.value))}
              />
            </label>
            <label className="text-sm">
              <span className="text-muted-foreground">v2 / canary %</span>
              <input
                type="number"
                min={0}
                max={100}
                className="mt-1 block w-24 rounded-md border bg-background px-2 py-1"
                value={canary}
                onChange={(e) => setCanary(Number(e.target.value))}
              />
            </label>
            <button
              type="button"
              className={btnPrimary}
              disabled={!!busy}
              onClick={() =>
                void run("weights", async () => {
                  const body = await setABWeights(id, [
                    { label: "v1", weight: stable },
                    { label: "v2", weight: canary },
                  ]);
                  toast.success(String(body.message ?? "Weights updated"));
                  await refresh();
                })
              }
            >
              {busy === "weights" ? "Updating…" : "Apply weights"}
            </button>
            <button
              type="button"
              className={btn}
              disabled={!!busy}
              onClick={() =>
                void run("promote", async () => {
                  const body = await promoteAB(id, "v2");
                  toast.success(String(body.message ?? "Promoted"));
                  await refresh();
                })
              }
            >
              Promote v2
            </button>
            <button
              type="button"
              className={btn}
              disabled={!!busy}
              onClick={() =>
                void run("rollback", async () => {
                  const body = await rollbackAB(id);
                  toast.success(String(body.message ?? "Rolled back"));
                  await refresh();
                })
              }
            >
              Rollback
            </button>
            <button
              type="button"
              className={btn}
              disabled={!!busy}
              onClick={() =>
                void run("observe", async () => {
                  const body = await observeAB(id, 40);
                  setObserve(body);
                  toast.success(String(body.message ?? "Observe done"));
                })
              }
            >
              {busy === "observe" ? "Sampling…" : "Observe ×40"}
            </button>
            <button type="button" className={btn} disabled={!!busy} onClick={() => void refresh()}>
              Refresh
            </button>
          </div>
        </section>
      )}

      {observe && (
        <section className="space-y-3 rounded-xl border p-4">
          <h3 className="font-semibold">Live traffic observe</h3>
          <p className="text-sm text-muted-foreground">
            {observe.url} · n={observe.n} · {observe.mode}
          </p>
          <div className="flex flex-wrap gap-3 text-sm">
            {Object.entries(observe.counts).map(([k, v]) => (
              <div key={k} className="rounded-lg border bg-muted/30 px-3 py-2 text-center">
                <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{k}</div>
                <div className="font-semibold tabular-nums">{v}</div>
                {observe.expected[k] != null && (
                  <div className="text-[10px] text-muted-foreground">
                    expected {observe.expected[k]}%
                  </div>
                )}
              </div>
            ))}
          </div>
          <div className="flex flex-wrap gap-1">
            {observe.samples.map((s, i) => (
              <span
                key={i}
                title={s.error || s.body || s.variant}
                className={`h-2.5 w-2.5 rounded-full ${
                  s.variant === "v2"
                    ? "bg-orange-500"
                    : s.variant === "v1"
                      ? "bg-sky-500"
                      : "bg-muted-foreground/40"
                }`}
              />
            ))}
          </div>
          <p className="text-xs text-muted-foreground">
            Each dot is one request to the observe path. Sky = v1, orange = v2.
          </p>
        </section>
      )}

      {experiments.length > 1 && (
        <section className="rounded-xl border p-4">
          <h3 className="mb-2 font-semibold">Experiments</h3>
          <ul className="space-y-1 text-sm">
            {experiments.map((e) => (
              <li key={e.id}>
                <button
                  type="button"
                  className={selected?.id === e.id ? "font-semibold underline" : "underline-offset-2 hover:underline"}
                  onClick={() => setSelected(e)}
                >
                  {e.name} ({e.host})
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
    </div>
  );
}
