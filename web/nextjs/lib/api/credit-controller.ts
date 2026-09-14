import { apiClient } from "@/lib/api/client";

export type InvoiceSummary = {
  invoice_id: string;
  supplier_name: string;
  received_at: string;
  amount_gbp: number | null;
  po_reference: string | null;
  scenario_hint: string;
  source: string;
  workflow: { run_id: string; status: string; updated_at: string } | null;
};

/** Whether mutating actions (AI generation, triage, approve) reach aivc-agents. */
export type CreditControllerWriteActions = {
  enabled: boolean;
  status: string;
  message: string;
};

/** demo = fixtures (no AIVC_BASE_URL); live = aivc-agents, `ok` false when unreachable. */
export type CreditControllerStatus = {
  mode: "demo" | "live";
  live_configured: boolean;
  ok: boolean;
  message?: string;
  base_url?: string;
  write_actions?: CreditControllerWriteActions;
};

export type CreditControllerSummary = {
  mode?: "demo" | "live";
  company?: string;
  message?: string;
  write_actions?: CreditControllerWriteActions;
  role: string;
  period: string;
  mailbox: { total_invoices: number; untriaged: number; total_gbp: number };
  queue: { awaiting_approval: number; items: { run_id: string; invoice_id: string }[] };
  outcomes: { posted: number; failed: number };
  by_scenario: Record<string, number>;
  playbook: { step: number; task: string; detail: string }[];
};

export async function creditControllerStatus() {
  const { data } = await apiClient.get<CreditControllerStatus>("/credit-controller/status");
  return data;
}

export async function listCreditInvoices() {
  const { data } = await apiClient.get<{
    count: number;
    total_gbp: number;
    invoices: InvoiceSummary[];
  }>("/credit-controller/invoices");
  return data;
}

export async function getCreditInvoice(invoiceId: string) {
  const { data } = await apiClient.get<{ document_text?: string } & InvoiceSummary>(
    `/credit-controller/invoices/${encodeURIComponent(invoiceId)}`
  );
  return data;
}

export async function creditControllerSummary() {
  const { data } = await apiClient.get<CreditControllerSummary>("/credit-controller/summary");
  return data;
}

export async function generateCreditInvoices(cadence: "weekly" | "monthly", count = 10) {
  const { data } = await apiClient.post<{ message?: string; batch?: unknown }>(
    "/credit-controller/invoices/generate",
    { cadence, count },
    { timeout: 180_000 }
  );
  return data;
}

export async function triageCreditInvoice(invoiceId: string) {
  const { data } = await apiClient.post<Record<string, unknown>>(
    "/credit-controller/triage",
    { invoice_id: invoiceId },
    { timeout: 180_000 }
  );
  return data;
}

export async function triageCreditBatch(invoiceIds: string[]) {
  const { data } = await apiClient.post<{
    triaged?: number;
    awaiting_approval?: number;
    succeeded?: number;
  }>("/credit-controller/triage/batch", { invoice_ids: invoiceIds }, { timeout: 300_000 });
  return data;
}

export async function creditControllerQueue() {
  const { data } = await apiClient.get<{ awaiting_approval?: unknown[] }>("/credit-controller/queue");
  return data;
}

export async function approveCreditRun(opts: {
  runId: string;
  approved: boolean;
  approver?: string;
  note?: string;
}) {
  const { data } = await apiClient.post<Record<string, unknown>>("/credit-controller/approve", {
    run_id: opts.runId,
    approved: opts.approved,
    approver: opts.approver ?? "s.oyelaran",
    note: opts.note ?? "",
  });
  return data;
}
