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
  type ABStatus,
} from "@/lib/api/ab-testing";

const btn =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 border border-border bg-background hover:bg-muted";
const btnPrimary =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 bg-primary text-primary-foreground hover:bg-primary/90";

const VARIANT_DOTS = ["bg-sky-500", "bg-orange-500", "bg-emerald-500", "bg-violet-500", "bg-pink-500"];

function modeLabel(st: ABStatus | null): string {
  if (!st) return "";
  switch (st.mode) {
    case "demo":
      return "Demo";
    case "live":
      return st.write_path === "mcp" ? "Live · wslproxy MCP" : "Live · wslproxy admin API";
    case "read_only":
      return "Read-only";
    default:
      return "Unavailable";
  }
}

function modeTone(st: ABStatus | null): string {
  if (!st) return "";
  if (st.mode === "live") return "border-emerald-500/30 bg-emerald-500/10 text-emerald-900 dark:text-emerald-100";
  if (st.mode === "demo") return "border-border bg-muted/40 text-muted-foreground";
  return "border-amber-500/30 bg-amber-500/10 text-amber-950 dark:text-amber-100";
}

export function AbTestAgentClient() {
  const [status, setStatus] = useState<ABStatus | null>(null);
  const [experiments, setExperiments] = useState<ABExperiment[]>([]);
  const [selected, setSelected] = useState<ABExperiment | null>(null);
  const [weights, setWeights] = useState<Record<string, number>>({});
  const [promoteLabel, setPromoteLabel] = useState("");
  const [observe, setObserve] = useState<ABObserveResult | null>(null);
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    try {
      setError("");
      const [st, list] = await Promise.all([abTestStatus(), listABExperiments()]);
      setStatus(st);
      const items = list.experiments ?? [];
      setExperiments(items);
      setSelected((prev) => items.find((e) => e.id === prev?.id) ?? items[0] ?? null);
    } catch (e) {
      setError(apiErrorMessage(e, "Failed to load AB Testing agent"));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  // Weight inputs mirror the rule's real backends.
  useEffect(() => {
    const backends = selected?.backends ?? [];
    setWeights(Object.fromEntries(backends.map((b) => [b.label, b.weight])));
    setPromoteLabel(backends[1]?.label ?? backends[0]?.label ?? "");
  }, [selected]);

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
  const writable = !!selected?.writable;
  const blockedReason = writable ? undefined : selected?.read_only_reason || "This experiment cannot be changed.";
  const backends = selected?.backends ?? [];
  const canSplit = writable && backends.length >= 2;
  const total = backends.reduce((sum, b) => sum + (weights[b.label] ?? 0), 0);
  const observeVariants = observe
    ? observe.variants ?? Object.keys(observe.counts).sort()
    : [];

  return (
    <div className="space-y-4 p-4">
      <header className="space-y-2">
        <h2 className="text-lg font-semibold">AB Testing</h2>
        <p className="text-sm text-muted-foreground">
          Weighted / canary traffic splits on wslproxy for{" "}
          <a
            className="underline underline-offset-2"
            href={status?.public_url ?? "https://abtesting.fictionally.org/"}
            target="_blank"
            rel="noreferrer"
          >
            {status?.demo_host ?? "abtesting.fictionally.org"}
          </a>
          . Writes go through wslproxy MCP when its tools are on, otherwise the wslproxy admin
          API.
        </p>
        {status && (
          <p className={`rounded-md border px-3 py-2 text-xs ${modeTone(status)}`}>
            <span className="font-semibold">{modeLabel(status)}</span> · {status.message}
          </p>
        )}
        {error && <p className="text-sm text-destructive">{error}</p>}
      </header>

      {selected && (
        <section className="space-y-3 rounded-xl border p-4">
          <div className="flex flex-wrap items-baseline justify-between gap-2">
            <div>
              <h3 className="font-semibold">{selected.host}</h3>
              <p className="text-xs text-muted-foreground">
                {selected.rule_id
                  ? `rule ${selected.rule_name ? `${selected.rule_name} · ` : ""}${selected.rule_id}`
                  : "no rule resolved"}{" "}
                · profile {selected.profile_id} · {selected.mode}
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

          {!writable && (
            <p className="rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-950 dark:text-amber-100">
              <span className="font-semibold">Read-only.</span> {blockedReason}
            </p>
          )}

          {backends.length > 0 ? (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-left text-[11px] uppercase tracking-wide text-muted-foreground">
                    <th className="py-1 pr-3 font-medium">Backend</th>
                    <th className="py-1 pr-3 font-medium">Address</th>
                    <th className="py-1 pr-3 font-medium">Weight on wslproxy</th>
                    {canSplit && <th className="py-1 font-medium">New weight %</th>}
                  </tr>
                </thead>
                <tbody>
                  {backends.map((b) => (
                    <tr key={b.label} className="border-t border-border">
                      <td className="py-1.5 pr-3">{b.label}</td>
                      <td className="py-1.5 pr-3 font-mono text-xs text-muted-foreground">
                        {b.address || "—"}
                      </td>
                      <td className="py-1.5 pr-3 tabular-nums">{b.weight}%</td>
                      {canSplit && (
                        <td className="py-1.5">
                          <input
                            type="number"
                            min={0}
                            max={100}
                            aria-label={`New weight for ${b.label}`}
                            className="w-24 rounded-md border bg-background px-2 py-1"
                            value={weights[b.label] ?? 0}
                            onChange={(e) =>
                              setWeights((w) => ({ ...w, [b.label]: Number(e.target.value) }))
                            }
                          />
                        </td>
                      )}
                    </tr>
                  ))}
                </tbody>
              </table>
              {canSplit && total !== 100 && (
                <p className="mt-1 text-xs text-muted-foreground">
                  Weights add up to {total}; wslproxy normalises them to 100.
                </p>
              )}
            </div>
          ) : (
            <p className="text-sm text-muted-foreground">No backends read from wslproxy.</p>
          )}

          <div className="flex flex-wrap items-end gap-3">
            <button
              type="button"
              className={btnPrimary}
              disabled={!!busy || !canSplit}
              title={blockedReason}
              onClick={() =>
                void run("weights", async () => {
                  const body = await setABWeights(
                    id,
                    backends.map((b) => ({ label: b.label, weight: weights[b.label] ?? 0 }))
                  );
                  toast.success(String(body.message ?? "Weights updated"));
                  await refresh();
                })
              }
            >
              {busy === "weights" ? "Updating…" : "Apply weights"}
            </button>
            {canSplit && (
              <select
                aria-label="Backend to promote"
                className="rounded-md border bg-background px-2 py-1.5 text-sm"
                value={promoteLabel}
                onChange={(e) => setPromoteLabel(e.target.value)}
              >
                {backends.map((b) => (
                  <option key={b.label} value={b.label}>
                    {b.label}
                  </option>
                ))}
              </select>
            )}
            <button
              type="button"
              className={btn}
              disabled={!!busy || !canSplit}
              title={blockedReason}
              onClick={() => {
                if (!confirm(`Send 100% of ${selected.host} traffic to ${promoteLabel}?`)) return;
                void run("promote", async () => {
                  const body = await promoteAB(id, promoteLabel);
                  toast.success(String(body.message ?? "Promoted"));
                  await refresh();
                });
              }}
            >
              {canSplit && promoteLabel ? `Promote ${promoteLabel}` : "Promote"}
            </button>
            <button
              type="button"
              className={btn}
              disabled={!!busy || !canSplit}
              title={blockedReason}
              onClick={() => {
                if (!confirm(`Remove the split on ${selected.host} and route to a single backend?`)) return;
                void run("rollback", async () => {
                  const body = await rollbackAB(id);
                  toast.success(String(body.message ?? "Rolled back"));
                  await refresh();
                });
              }}
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
            {observeVariants.map((k, i) => (
              <div key={k} className="rounded-lg border bg-muted/30 px-3 py-2 text-center">
                <div className="flex items-center justify-center gap-1.5 text-[10px] uppercase tracking-wide text-muted-foreground">
                  <span className={`h-2 w-2 rounded-full ${VARIANT_DOTS[i % VARIANT_DOTS.length]}`} />
                  {k}
                </div>
                <div className="font-semibold tabular-nums">{observe.counts[k] ?? 0}</div>
                {observe.expected[k] != null && (
                  <div className="text-[10px] text-muted-foreground">
                    wslproxy weight {observe.expected[k]}%
                  </div>
                )}
              </div>
            ))}
          </div>
          <div className="flex flex-wrap gap-1">
            {observe.samples.map((s, i) => {
              const idx = observeVariants.indexOf(s.variant || (s.error ? "error" : "unknown"));
              return (
                <span
                  key={i}
                  title={s.error || s.body || s.variant}
                  className={`h-2.5 w-2.5 rounded-full ${
                    s.variant && idx >= 0 ? VARIANT_DOTS[idx % VARIANT_DOTS.length] : "bg-muted-foreground/40"
                  }`}
                />
              );
            })}
          </div>
          <p className="text-xs text-muted-foreground">
            Each dot is one request to the observe path, coloured by the variant it reported.
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
