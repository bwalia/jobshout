"use client";

import { useCallback, useEffect, useState } from "react";
import { toast } from "sonner";
import { apiErrorMessage } from "@/lib/api/client";
import {
  listSimproInvoices,
  listSimproPayments,
  simproFGasReport,
  simproReconcilePreview,
  simproStatus,
  simproSummary,
  type RefrigerantUsage,
  type SimproInvoice,
  type SimproPayment,
  type SimproSummary,
} from "@/lib/api/simpro-payments";

const btn =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 border border-border bg-background hover:bg-muted";
const btnPrimary =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 bg-primary text-primary-foreground hover:bg-primary/90";

type Tab = "invoices" | "payments" | "fgas" | "playbook";

export function SimproPaymentsAgentClient() {
  const [tab, setTab] = useState<Tab>("invoices");
  const [summary, setSummary] = useState<SimproSummary | null>(null);
  const [invoices, setInvoices] = useState<SimproInvoice[]>([]);
  const [payments, setPayments] = useState<SimproPayment[]>([]);
  const [fgas, setFgas] = useState<RefrigerantUsage[]>([]);
  const [mode, setMode] = useState("demo");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");
  const [reconcileNote, setReconcileNote] = useState("");

  const refresh = useCallback(async () => {
    try {
      setError("");
      const [st, sum, inv, pay, fg] = await Promise.all([
        simproStatus(),
        simproSummary(),
        listSimproInvoices(),
        listSimproPayments(),
        simproFGasReport(),
      ]);
      setMode(String(st.mode ?? sum.mode ?? "demo"));
      setSummary(sum);
      setInvoices(inv.invoices ?? []);
      setPayments(pay.payments ?? []);
      setFgas(fg.events ?? []);
    } catch (err) {
      setError(apiErrorMessage(err, "Could not load Simpro data."));
    }
  }, []);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const run = async (key: string, fn: () => Promise<void>) => {
    setBusy(key);
    try {
      await fn();
      await refresh();
    } catch (err) {
      toast.error(apiErrorMessage(err, "That action failed."));
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="flex h-full min-h-0 flex-col gap-4 overflow-auto p-4">
      <header className="rounded-xl border bg-card p-4">
        <div className="flex flex-wrap items-start justify-between gap-3">
          <div>
            <h2 className="text-lg font-semibold tracking-tight">
              Simpro Payments — {summary?.company ?? "Field service AR"}
            </h2>
            <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
              Invoice aging, payment status, and UK F-Gas refrigerant compliance — demo
              fixtures always available; live Simpro reads when an API key is configured.
            </p>
            <p className="mt-2 text-xs text-muted-foreground">
              Mode: <span className="font-medium text-foreground">{mode}</span>
              {summary?.period ? ` · ${summary.period}` : null}
            </p>
          </div>
          <div className="flex flex-wrap gap-3 text-sm">
            <Stat label="Open" value={String(summary?.mailbox.open ?? "—")} />
            <Stat label="Overdue" value={String(summary?.mailbox.overdue ?? "—")} />
            <Stat
              label="Outstanding"
              value={`£${(summary?.mailbox.outstanding_gbp ?? 0).toLocaleString("en-GB")}`}
            />
            <Stat label="F-Gas gaps" value={String(summary?.fgas.gaps ?? "—")} />
          </div>
        </div>
        {summary?.write_actions && (
          <p className="mt-3 rounded-md border border-amber-500/30 bg-amber-500/10 px-3 py-2 text-sm text-amber-950 dark:text-amber-100">
            Writes: {summary.write_actions.message}
          </p>
        )}
        {error && (
          <p className="mt-3 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        )}
      </header>

      <div className="flex flex-wrap items-center gap-2">
        {(
          [
            ["invoices", "Invoices"],
            ["payments", "Payments"],
            ["fgas", "F-Gas"],
            ["playbook", "Month-end"],
          ] as const
        ).map(([id, label]) => (
          <button
            key={id}
            type="button"
            className={tab === id ? btnPrimary : btn}
            onClick={() => setTab(id)}
          >
            {label}
          </button>
        ))}
        <button
          type="button"
          className={btn}
          disabled={!!busy}
          onClick={() =>
            void run("reconcile", async () => {
              const body = await simproReconcilePreview();
              setReconcileNote(
                typeof body.message === "string"
                  ? body.message
                  : "Reconciliation preview loaded (read-only)."
              );
              toast.success("Reconcile preview ready");
            })
          }
        >
          {busy === "reconcile" ? "Working…" : "Reconcile preview"}
        </button>
        <button type="button" className={btn} disabled={!!busy} onClick={() => void refresh()}>
          Refresh
        </button>
      </div>

      {reconcileNote && (
        <p className="rounded-md border bg-muted/40 px-3 py-2 text-sm">{reconcileNote}</p>
      )}

      {tab === "invoices" && (
        <section className="overflow-hidden rounded-xl border">
          <table className="w-full text-left text-sm">
            <thead className="border-b bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-2">Invoice</th>
                <th className="px-3 py-2">Customer</th>
                <th className="px-3 py-2">Due</th>
                <th className="px-3 py-2">Balance</th>
                <th className="px-3 py-2">Aging</th>
                <th className="px-3 py-2">Status</th>
              </tr>
            </thead>
            <tbody>
              {invoices.map((inv) => (
                <tr key={inv.id} className="border-b last:border-0">
                  <td className="px-3 py-2 font-medium">{inv.number}</td>
                  <td className="px-3 py-2">{inv.customer}</td>
                  <td className="px-3 py-2 tabular-nums">{inv.due_on}</td>
                  <td className="px-3 py-2 tabular-nums">
                    £{inv.balance_gbp.toLocaleString("en-GB")}
                  </td>
                  <td className="px-3 py-2">{inv.aging_bucket}</td>
                  <td className="px-3 py-2 capitalize">{inv.status}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {summary?.aging && (
            <div className="grid grid-cols-2 gap-2 border-t bg-muted/20 p-3 sm:grid-cols-5">
              {Object.entries(summary.aging).map(([bucket, amt]) => (
                <div key={bucket} className="rounded-md border bg-background px-2 py-1.5 text-xs">
                  <div className="text-muted-foreground">{bucket}</div>
                  <div className="font-semibold tabular-nums">
                    £{Number(amt).toLocaleString("en-GB")}
                  </div>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      {tab === "payments" && (
        <section className="overflow-hidden rounded-xl border">
          <table className="w-full text-left text-sm">
            <thead className="border-b bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-2">Received</th>
                <th className="px-3 py-2">Customer</th>
                <th className="px-3 py-2">Amount</th>
                <th className="px-3 py-2">Method</th>
                <th className="px-3 py-2">Invoice</th>
                <th className="px-3 py-2">Reference</th>
              </tr>
            </thead>
            <tbody>
              {payments.map((p) => (
                <tr key={p.id} className="border-b last:border-0">
                  <td className="px-3 py-2 tabular-nums">{p.received_on}</td>
                  <td className="px-3 py-2">{p.customer}</td>
                  <td className="px-3 py-2 tabular-nums">
                    £{p.amount_gbp.toLocaleString("en-GB")}
                  </td>
                  <td className="px-3 py-2">{p.method}</td>
                  <td className="px-3 py-2">{p.invoice_id || "— unallocated —"}</td>
                  <td className="px-3 py-2 text-muted-foreground">{p.reference}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </section>
      )}

      {tab === "fgas" && (
        <section className="space-y-3">
          <p className="max-w-3xl text-sm text-muted-foreground">
            {summary?.fgas.narrative}
          </p>
          <div className="flex flex-wrap gap-3 text-sm">
            <Stat label="Events" value={String(summary?.fgas.events ?? 0)} />
            <Stat label="Gaps" value={String(summary?.fgas.gaps ?? 0)} />
            <Stat label="Kg used" value={String(summary?.fgas.kg_used ?? 0)} />
            <Stat label="Kg recovered" value={String(summary?.fgas.kg_recovered ?? 0)} />
          </div>
          <div className="overflow-hidden rounded-xl border">
            <table className="w-full text-left text-sm">
              <thead className="border-b bg-muted/40 text-xs uppercase tracking-wide text-muted-foreground">
                <tr>
                  <th className="px-3 py-2">Date</th>
                  <th className="px-3 py-2">Job</th>
                  <th className="px-3 py-2">Site</th>
                  <th className="px-3 py-2">Tech</th>
                  <th className="px-3 py-2">Gas</th>
                  <th className="px-3 py-2">Used</th>
                  <th className="px-3 py-2">Recovered</th>
                  <th className="px-3 py-2">Notes</th>
                </tr>
              </thead>
              <tbody>
                {fgas.map((e) => (
                  <tr
                    key={e.id}
                    className={`border-b last:border-0 ${e.has_gap ? "bg-destructive/5" : ""}`}
                  >
                    <td className="px-3 py-2 tabular-nums">{e.date}</td>
                    <td className="px-3 py-2">{e.job_ref}</td>
                    <td className="px-3 py-2">{e.site}</td>
                    <td className="px-3 py-2">{e.technician}</td>
                    <td className="px-3 py-2 font-medium">{e.gas_type}</td>
                    <td className="px-3 py-2 tabular-nums">{e.kg_used}</td>
                    <td className="px-3 py-2 tabular-nums">{e.kg_recovered}</td>
                    <td className="px-3 py-2 text-muted-foreground">
                      {e.has_gap ? "⚠ " : ""}
                      {e.notes}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      {tab === "playbook" && (
        <section className="grid gap-3 md:grid-cols-2">
          {(summary?.playbook ?? []).map((step) => (
            <div key={step.step} className="rounded-xl border bg-card p-4">
              <div className="mb-1 font-mono text-[10px] text-muted-foreground">
                {String(step.step).padStart(2, "0")}
              </div>
              <h3 className="font-semibold">{step.task}</h3>
              <p className="mt-1 text-sm text-muted-foreground">{step.detail}</p>
            </div>
          ))}
          <div className="rounded-xl border border-dashed p-4 text-sm text-muted-foreground md:col-span-2">
            Posting receipts, creating invoices, or adjusting stock in Simpro stays a talk
            track until scoped with the customer and an API key — this UI never pretends
            writes are live.
          </div>
        </section>
      )}
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-[4.5rem] rounded-lg border bg-muted/30 px-3 py-2 text-center">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="font-semibold tabular-nums">{value}</div>
    </div>
  );
}
