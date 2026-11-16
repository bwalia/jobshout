import { API_BASE, failure, headers, type Viewer } from "@/lib/insights";

export type HireOrg = {
  id: string;
  name: string;
  slug: string;
  created_at: string;
};

export type Requisition = {
  id: string;
  org_id: string;
  title: string;
  description: string;
  kind: string;
  status: string;
  location: string;
  remote_ok: boolean;
  skills: string[];
  created_at: string;
};

export async function listHireOrgs(viewer: Viewer): Promise<HireOrg[]> {
  const res = await fetch(`${API_BASE}/api/v1/hire/orgs`, {
    cache: "no-store",
    headers: headers(viewer),
  });
  if (res.status === 404) {
    throw new Error("Linked Hire is not enabled on this ring (HIRE_ENABLED).");
  }
  if (!res.ok) throw await failure(res, "Failed to list hire orgs");
  const body = (await res.json()) as { data: HireOrg[] };
  return body.data ?? [];
}

export async function createHireOrg(
  viewer: Viewer,
  input: { name: string; slug: string },
): Promise<HireOrg> {
  const res = await fetch(`${API_BASE}/api/v1/hire/orgs`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
  });
  if (res.status === 404) {
    throw new Error("Linked Hire is not enabled on this ring (HIRE_ENABLED).");
  }
  if (!res.ok) throw await failure(res, "Failed to create hire org");
  const body = (await res.json()) as { data: HireOrg };
  return body.data;
}

export async function listRequisitions(viewer: Viewer, orgId: string): Promise<Requisition[]> {
  const res = await fetch(`${API_BASE}/api/v1/hire/orgs/${orgId}/requisitions`, {
    cache: "no-store",
    headers: headers(viewer),
  });
  if (res.status === 404) {
    throw new Error("Linked Hire is not enabled on this ring (HIRE_ENABLED).");
  }
  if (!res.ok) throw await failure(res, "Failed to list requisitions");
  const body = (await res.json()) as { data: Requisition[] };
  return body.data ?? [];
}

export async function createRequisition(
  viewer: Viewer,
  orgId: string,
  input: {
    title: string;
    description?: string;
    kind?: string;
    location?: string;
    remote_ok?: boolean;
    skills?: string[];
  },
): Promise<Requisition> {
  const res = await fetch(`${API_BASE}/api/v1/hire/orgs/${orgId}/requisitions`, {
    method: "POST",
    headers: headers(viewer),
    body: JSON.stringify(input),
  });
  if (res.status === 404) {
    throw new Error("Linked Hire is not enabled on this ring (HIRE_ENABLED).");
  }
  if (!res.ok) throw await failure(res, "Failed to create requisition");
  const body = (await res.json()) as { data: Requisition };
  return body.data;
}
