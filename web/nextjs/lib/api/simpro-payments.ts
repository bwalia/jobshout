import { apiClient } from "@/lib/api/client";

export type SimproInvoice = {
  id: string;
  number: string;
  customer: string;
  site: string;
  job_ref: string;
  issued_on: string;
  due_on: string;
  amount_gbp: number;
  paid_gbp: number;
  balance_gbp: number;
  status: string;
  aging_bucket: string;
  source: string;
};

export type SimproPayment = {
  id: string;
  invoice_id: string;
  customer: string;
  received_on: string;
  amount_gbp: number;
  method: string;
  reference: string;
  source: string;
};

export type RefrigerantUsage = {
  id: string;
  date: string;
  job_ref: string;
  customer: string;
  site: string;
  technician: string;
  gas_type: string;
  kg_used: number;
  kg_recovered: number;
  kg_topped_up: number;
  cylinder_ref: string;
  notes: string;
  has_gap: boolean;
  source: string;
};

export type SimproSummary = {
  role: string;
  period: string;
  mode: string;
  company: string;
  mailbox: {
    total_invoices: number;
    open: number;
    overdue: number;
    total_gbp: number;
    outstanding_gbp: number;
  };
  aging: Record<string, number>;
  payments: {
    received_this_period: number;
    received_gbp: number;
    unallocated_gbp: number;
  };
  fgas: {
    events: number;
    gaps: number;
    kg_used: number;
    kg_recovered: number;
    by_gas_type: Record<string, number>;
    narrative: string;
  };
  playbook: { step: number; task: string; detail: string }[];
  write_actions: { enabled: boolean; status: string; message: string };
  generated_at: string;
};

export async function simproStatus() {
  const { data } = await apiClient.get<Record<string, unknown>>("/simpro-payments/status");
  return data;
}

export async function simproSummary() {
  const { data } = await apiClient.get<SimproSummary>("/simpro-payments/summary");
  return data;
}

export async function listSimproInvoices() {
  const { data } = await apiClient.get<{
    mode: string;
    count: number;
    invoices: SimproInvoice[];
  }>("/simpro-payments/invoices");
  return data;
}

export async function listSimproPayments() {
  const { data } = await apiClient.get<{
    mode: string;
    count: number;
    payments: SimproPayment[];
  }>("/simpro-payments/payments");
  return data;
}

export async function simproAging() {
  const { data } = await apiClient.get<Record<string, unknown>>("/simpro-payments/aging");
  return data;
}

export async function simproReconcilePreview() {
  const { data } = await apiClient.get<Record<string, unknown>>("/simpro-payments/reconcile-preview");
  return data;
}

export async function simproMonthEnd() {
  const { data } = await apiClient.get<Record<string, unknown>>("/simpro-payments/month-end");
  return data;
}

export async function simproFGasReport() {
  const { data } = await apiClient.get<{
    mode: string;
    period: string;
    company: string;
    stats: SimproSummary["fgas"];
    events: RefrigerantUsage[];
    gaps: RefrigerantUsage[];
    export_hint: string;
    write_actions: SimproSummary["write_actions"];
  }>("/simpro-payments/fgas");
  return data;
}
