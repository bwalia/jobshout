/**
 * Identity lives in OpsAPI CRM only — never in NebulaDB vectors.
 * Uses the workspace API key already mounted as OPSAPI_API_KEY.
 */

type LeadInput = {
  first_name: string;
  last_name?: string;
  email: string;
  phone?: string;
  company_name?: string;
  job_title?: string;
  notes?: string;
  role: "recruiter" | "candidate";
  nebula_doc_id?: string;
  session_id?: string;
};

function config(): { base: string; namespace: string; apiKey: string } | null {
  const base = process.env.OPSAPI_URL?.trim().replace(/\/$/, "");
  const namespace = process.env.OPSAPI_NAMESPACE?.trim();
  const apiKey = process.env.OPSAPI_API_KEY?.trim();
  if (!base || !namespace || !apiKey) return null;
  return { base, namespace, apiKey };
}

export function crmEnabled(): boolean {
  return config() !== null;
}

export async function upsertJobSupportLead(
  input: LeadInput,
): Promise<{ uuid: string } | null> {
  const cfg = config();
  if (!cfg) return null;

  const email = input.email.trim().toLowerCase();
  if (!email) return null;

  const body = {
    first_name: input.first_name || email.split("@")[0],
    last_name: input.last_name || "",
    email,
    phone: input.phone || undefined,
    company_name: input.company_name || undefined,
    job_title: input.job_title || undefined,
    source: "jobshout_job_support",
    channel: "web_chat",
    status: "new",
    priority: "medium",
    notes: input.notes || undefined,
    metadata: {
      role: input.role,
      nebula_doc_id: input.nebula_doc_id ?? null,
      session_id: input.session_id ?? null,
      product: "jobshout.com",
    },
  };

  const res = await fetch(`${cfg.base}/api/v2/crm/leads`, {
    method: "POST",
    headers: {
      "content-type": "application/json",
      accept: "application/json",
      authorization: `Bearer ${cfg.apiKey}`,
      "x-namespace-slug": cfg.namespace,
    },
    body: JSON.stringify(body),
    cache: "no-store",
  });

  if (!res.ok) {
    const text = await res.text().catch(() => "");
    throw new Error(`OpsAPI CRM ${res.status}: ${text.slice(0, 240)}`);
  }

  const json = (await res.json()) as { data?: { uuid?: string }; uuid?: string };
  const uuid = json.data?.uuid ?? json.uuid;
  return uuid ? { uuid } : null;
}
