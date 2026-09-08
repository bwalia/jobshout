"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";
import { apiErrorMessage } from "@/lib/api/client";
import {
  approveCreditRun,
  creditControllerSummary,
  generateCreditInvoices,
  getCreditInvoice,
  listCreditInvoices,
  triageCreditBatch,
  triageCreditInvoice,
  type CreditControllerSummary,
  type InvoiceSummary,
} from "@/lib/api/credit-controller";

const SCENARIO_LABELS: Record<string, string> = {
  STRAIGHT_THROUGH: "Straight-through",
  PRICE_VARIANCE: "Price variance",
  QUANTITY_VARIANCE: "Qty variance",
  NO_PO: "No PO",
  DUPLICATE_SUSPECT: "Duplicate",
  BANK_DETAIL_CHANGE: "Bank change",
};

const PROCESS = [
  "Intake",
  "Extract",
  "3-way match",
  "Classify",
  "Policy",
  "Approve",
  "Post",
  "Audit",
];

const btn =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 border border-border bg-background hover:bg-muted";
const btnPrimary =
  "rounded-md px-3 py-1.5 text-sm font-medium disabled:opacity-50 bg-primary text-primary-foreground hover:bg-primary/90";

export function CreditControllerAgentClient() {
  const [invoices, setInvoices] = useState<InvoiceSummary[]>([]);
  const [summary, setSummary] = useState<CreditControllerSummary | null>(null);
  const [selected, setSelected] = useState<string>("");
  const [doc, setDoc] = useState<string>("");
  const [filter, setFilter] = useState<"all" | "untriaged" | "awaiting">("all");
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState("");

  const refresh = useCallback(async () => {
    try {
      setError("");
      const [list, sum] = await Promise.all([listCreditInvoices(), creditControllerSummary()]);
      setInvoices(list.invoices ?? []);
      setSummary(sum);
      if (!selected && list.invoices?.[0]) {
        setSelected(list.invoices[0].invoice_id);
      }
    } catch (err) {
      setError(apiErrorMessage(err));
    }
  }, [selected]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  useEffect(() => {
    if (!selected) return;
    void (async () => {
      try {
        const detail = await getCreditInvoice(selected);
        setDoc(detail.document_text ?? "");
      } catch (err) {
        setDoc("");
        toast.error(apiErrorMessage(err));
      }
    })();
  }, [selected]);

  const filtered = useMemo(() => {
    if (filter === "untriaged") return invoices.filter((i) => !i.workflow);
    if (filter === "awaiting") {
      return invoices.filter((i) => i.workflow?.status === "awaiting_approval");
    }
    return invoices;
  }, [invoices, filter]);

  const run = async (key: string, fn: () => Promise<void>) => {
    setBusy(key);
    try {
      await fn();
      await refresh();
    } catch (err) {
      toast.error(apiErrorMessage(err));
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
              Credit Controller — {summary?.period ?? "AP mailbox"}
            </h2>
            <p className="mt-1 max-w-2xl text-sm text-muted-foreground">
              Supplier invoice mailbox, AI weekly/monthly batches, and the full exception
              workflow from intake through audit.
            </p>
          </div>
          <div className="flex flex-wrap gap-3 text-sm">
            <Stat label="Mailbox" value={String(summary?.mailbox.total_invoices ?? invoices.length)} />
            <Stat label="Untriaged" value={String(summary?.mailbox.untriaged ?? "—")} />
            <Stat label="Awaiting" value={String(summary?.queue.awaiting_approval ?? "—")} />
            <Stat
              label="Portfolio"
              value={`£${(summary?.mailbox.total_gbp ?? 0).toLocaleString("en-GB")}`}
            />
          </div>
        </div>
        {error && (
          <p className="mt-3 rounded-md border border-destructive/40 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {error}
          </p>
        )}
      </header>

      <section className="grid grid-cols-2 gap-2 sm:grid-cols-4 lg:grid-cols-8">
        {PROCESS.map((step, i) => (
          <div key={step} className="rounded-lg border bg-muted/30 px-2 py-2 text-xs">
            <div className="mb-1 font-mono text-[10px] text-muted-foreground">
              {String(i + 1).padStart(2, "0")}
            </div>
            <div className="font-medium">{step}</div>
          </div>
        ))}
      </section>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex gap-1">
          {(["all", "untriaged", "awaiting"] as const).map((f) => (
            <button
              key={f}
              type="button"
              className={filter === f ? btnPrimary : btn}
              onClick={() => setFilter(f)}
            >
              {f === "all" ? "All" : f === "untriaged" ? "Untriaged" : "Awaiting"}
            </button>
          ))}
        </div>
        <div className="flex flex-wrap gap-2">
          <button type="button" className={btn} disabled={!!busy} onClick={() => void refresh()}>
            Refresh
          </button>
          <button
            type="button"
            className={btn}
            disabled={!!busy}
            onClick={() =>
              void run("weekly", async () => {
                const res = await generateCreditInvoices("weekly", 10);
                toast.success(res.message ?? "Weekly batch generated");
              })
            }
          >
            {busy === "weekly" ? "…" : "AI: 10 weekly"}
          </button>
          <button
            type="button"
            className={btn}
            disabled={!!busy}
            onClick={() =>
              void run("monthly", async () => {
                const res = await generateCreditInvoices("monthly", 10);
                toast.success(res.message ?? "Monthly batch generated");
              })
            }
          >
            {busy === "monthly" ? "…" : "AI: 10 monthly"}
          </button>
          <button
            type="button"
            className={btnPrimary}
            disabled={!!busy}
            onClick={() =>
              void run("batch", async () => {
                const ids = invoices.filter((i) => !i.workflow).map((i) => i.invoice_id).slice(0, 15);
                if (!ids.length) {
                  toast.message("No untriaged invoices");
                  return;
                }
                const res = await triageCreditBatch(ids);
                toast.success(
                  `Triaged ${res.triaged ?? ids.length} · awaiting ${res.awaiting_approval ?? 0}`
                );
              })
            }
          >
            {busy === "batch" ? "Triaging…" : "Triage untriaged"}
          </button>
          <button
            type="button"
            className={btnPrimary}
            disabled={!!busy}
            onClick={() =>
              void run("month_end", async () => {
                await generateCreditInvoices("monthly", 10);
                const list = await listCreditInvoices();
                const ids = (list.invoices ?? [])
                  .filter((i) => !i.workflow)
                  .map((i) => i.invoice_id)
                  .slice(0, 15);
                if (ids.length) await triageCreditBatch(ids);
                toast.success("Month-end run complete — approve exceptions in the queue");
              })
            }
          >
            {busy === "month_end" ? "Running…" : "Full month-end"}
          </button>
        </div>
      </div>

      <div className="grid min-h-0 flex-1 gap-4 lg:grid-cols-[1.4fr_1fr]">
        <div className="overflow-auto rounded-xl border">
          <table className="w-full text-left text-sm">
            <thead className="sticky top-0 bg-muted/80 text-xs uppercase tracking-wide text-muted-foreground">
              <tr>
                <th className="px-3 py-2">Invoice</th>
                <th className="px-3 py-2">Supplier</th>
                <th className="px-3 py-2">Received</th>
                <th className="px-3 py-2">Amount</th>
                <th className="px-3 py-2">Scenario</th>
                <th className="px-3 py-2">Status</th>
                <th className="px-3 py-2" />
              </tr>
            </thead>
            <tbody>
              {filtered.map((inv) => (
                <tr
                  key={inv.invoice_id}
                  className={`cursor-pointer border-t hover:bg-muted/40 ${
                    selected === inv.invoice_id ? "bg-muted/60" : ""
                  }`}
                  onClick={() => setSelected(inv.invoice_id)}
                >
                  <td className="px-3 py-2 font-mono text-xs">
                    {inv.invoice_id}
                    {inv.source === "generated" && (
                      <span className="ml-2 rounded bg-muted px-1.5 py-0.5 text-[10px] uppercase">
                        AI
                      </span>
                    )}
                  </td>
                  <td className="px-3 py-2">{inv.supplier_name}</td>
                  <td className="px-3 py-2">{inv.received_at}</td>
                  <td className="px-3 py-2">
                    {inv.amount_gbp != null
                      ? `£${inv.amount_gbp.toLocaleString("en-GB")}`
                      : "—"}
                  </td>
                  <td className="px-3 py-2">
                    <span className="rounded border px-1.5 py-0.5 text-[11px]">
                      {SCENARIO_LABELS[inv.scenario_hint] ?? inv.scenario_hint}
                    </span>
                  </td>
                  <td className="px-3 py-2 font-mono text-xs">
                    {inv.workflow?.status ?? "new"}
                  </td>
                  <td className="px-3 py-2">
                    <div className="flex gap-1">
                      <button
                        type="button"
                        className="text-xs text-primary hover:underline disabled:opacity-50"
                        disabled={!!busy}
                        onClick={(e) => {
                          e.stopPropagation();
                          void run(`triage-${inv.invoice_id}`, async () => {
                            const res = await triageCreditInvoice(inv.invoice_id);
                            toast.success(`${inv.invoice_id} → ${String(res.status ?? "done")}`);
                          });
                        }}
                      >
                        Triage
                      </button>
                      {inv.workflow?.status === "awaiting_approval" && inv.workflow.run_id && (
                        <button
                          type="button"
                          className="text-xs text-primary hover:underline disabled:opacity-50"
                          disabled={!!busy}
                          onClick={(e) => {
                            e.stopPropagation();
                            void run(`approve-${inv.workflow!.run_id}`, async () => {
                              await approveCreditRun({
                                runId: inv.workflow!.run_id,
                                approved: true,
                                note: "approved from Credit Controller tab",
                              });
                              toast.success(`Approved ${inv.invoice_id}`);
                            });
                          }}
                        >
                          Approve
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>

        <aside className="space-y-4 rounded-xl border p-4">
          <div>
            <h3 className="text-sm font-semibold">{selected || "Select an invoice"}</h3>
            {doc ? (
              <pre className="mt-2 max-h-72 overflow-auto rounded-md bg-muted/50 p-3 font-mono text-[11px] leading-relaxed">
                {doc}
              </pre>
            ) : (
              <p className="mt-2 text-sm text-muted-foreground">
                Click a row to preview the supplier document.
              </p>
            )}
          </div>
          {summary?.playbook && (
            <div>
              <h3 className="text-sm font-semibold">Playbook</h3>
              <ol className="mt-2 list-decimal space-y-2 pl-4 text-sm">
                {summary.playbook.map((p) => (
                  <li key={p.step}>
                    <strong className="block">{p.task}</strong>
                    <span className="text-muted-foreground">{p.detail}</span>
                  </li>
                ))}
              </ol>
            </div>
          )}
        </aside>
      </div>
    </div>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-[4.5rem] rounded-lg border px-3 py-2">
      <div className="text-[10px] uppercase tracking-wide text-muted-foreground">{label}</div>
      <div className="text-base font-semibold tabular-nums">{value}</div>
    </div>
  );
}
